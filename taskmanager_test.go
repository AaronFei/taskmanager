package taskmanager

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestTaskManager_BasicTask(t *testing.T) {
	tests := []struct {
		name              string
		tf                TaskFunc
		expectedStatus    interface{}
		expectedOperation TaskOperation_t
	}{
		{
			name:              "add_event",
			tf:                nil,
			expectedStatus:    nil,
			expectedOperation: OPERATION_START,
		},
		{
			name: "send event",
			tf: func(_ context.Context, eventChan chan<- StatusEvent_t) error {
				eventChan <- StatusEvent_t{
					Status:    "status1",
					Operation: OPERATION_STOP,
					Func:      nil,
				}
				return nil
			},
			expectedStatus:    "status1",
			expectedOperation: OPERATION_STOP,
		},
		{
			name: "self restart",
			tf: func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
				eventChan <- StatusEvent_t{
					Status:    "status1",
					Operation: OPERATION_RESTART,
					Func: func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
						eventChan <- StatusEvent_t{
							Status:    "status2",
							Operation: OPERATION_NONE,
							Func:      nil,
						}

						for {
							select {
							case <-ctx.Done():
								return nil
							}
						}
					},
				}

				for {
					select {
					case <-ctx.Done():
						return nil
					}
				}
			},
			expectedStatus:    "status2",
			expectedOperation: OPERATION_START,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgRecord := make([]string, 0)
			isTestError := false

			defer func() {
				if isTestError {
					for _, msg := range msgRecord {
						fmt.Printf("msg: %v\n", msg)
					}
				}
			}()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tm := NewTaskManager()
			tm.AddTask(tt.name, tt.tf)
			err := tm.RunTask(tt.name)
			if err != nil {
				t.Errorf("run task error: %v", err)
			}

			go func() {
				for {
					select {
					case <-ctx.Done():
						return
					case msg := <-tm.GetEventMsg():
						msgRecord = append(msgRecord, msg.Msg)
					}
				}
			}()

			timeout := time.NewTimer(5 * time.Second)

		WAIT_TASK_STATUS:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task status timeout")
					isTestError = true
					return
				default:
					if taskStatus, err := tm.GetTaskStatus(tt.name); err == nil && taskStatus == tt.expectedStatus {
						break WAIT_TASK_STATUS
					} else {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}

			timeout.Reset(5 * time.Second)
		WAIT_TASK_OPERATION:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task operation timeout")
					isTestError = true
					return
				default:
					if taskOperation, err := tm.GetTaskOperation(tt.name); err == nil && taskOperation == tt.expectedOperation {
						break WAIT_TASK_OPERATION
					} else {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}

			tm.DeleteTask(tt.name)

			timeout.Reset(5 * time.Second)
		WAIT_TASK_DELETE:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait delete timeout")
					isTestError = true
					return
				default:
					if !tm.IsTaskExist(tt.name) {
						break WAIT_TASK_DELETE
					} else {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}

			tm.StopTaskManager(true)
		})
	}
}

func TestTaskManager_StatusChange(t *testing.T) {
	externalEventChan := make(chan StatusEvent_t, 10)

	tf := func(_ context.Context, eventChan chan<- StatusEvent_t) error {
		for event := range externalEventChan {
			eventChan <- event
		}
		return nil
	}

	tests := []struct {
		name              string
		statusEvent       StatusEvent_t
		expectedStatus    interface{}
		expectedOperation TaskOperation_t
	}{
		{
			name: "change to status1",
			statusEvent: StatusEvent_t{
				Status: "status1",
			},
			expectedStatus:    "status1",
			expectedOperation: OPERATION_START,
		},
		{
			name: "change to status2",
			statusEvent: StatusEvent_t{
				Status: "status2",
			},
			expectedStatus:    "status2",
			expectedOperation: OPERATION_START,
		},
		{
			name: "change to status3",
			statusEvent: StatusEvent_t{
				Status: "status3",
			},
			expectedStatus:    "status3",
			expectedOperation: OPERATION_START,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgRecord := make([]string, 0)
	isTestError := false

	defer func() {
		if isTestError {
			for _, msg := range msgRecord {
				fmt.Printf("msg: %v\n", msg)
			}
		}
	}()

	tm := NewTaskManager()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-tm.GetEventMsg():
				msgRecord = append(msgRecord, msg.Msg)
			}
		}
	}()

	tm.AddTask("status_test", tf)
	tm.RunTask("status_test")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			externalEventChan <- tt.statusEvent

			timeout := time.NewTimer(5 * time.Second)

		WAIT_TASK_STATUS:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task status timeout")
					isTestError = true
					return
				default:
					if taskStatus, err := tm.GetTaskStatus("status_test"); err == nil && reflect.DeepEqual(taskStatus, tt.expectedStatus) {
						break WAIT_TASK_STATUS
					} else {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}

			timeout.Reset(5 * time.Second)
		WAIT_TASK_OPERATION:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task operation timeout")
					isTestError = true
					return
				default:
					if taskOperation, err := tm.GetTaskOperation("status_test"); err == nil && taskOperation == tt.expectedOperation {
						break WAIT_TASK_OPERATION
					} else {
						time.Sleep(1 * time.Millisecond)
					}
				}
			}
		})
	}

	tm.StopTaskManager(true)
}

func TestTaskManager_OperationChange(t *testing.T) {
	tests := []struct {
		name              string
		operation         []TaskOperation_t
		idx               int
		expectResult      []string
		externalChan      chan string
		externalEventChan chan StatusEvent_t
	}{
		{
			name:              "stop task",
			operation:         []TaskOperation_t{OPERATION_STOP},
			idx:               0,
			expectResult:      []string{"start", "done"},
			externalChan:      make(chan string, 10),
			externalEventChan: make(chan StatusEvent_t, 10),
		},
		{
			name:              "restart task",
			operation:         []TaskOperation_t{OPERATION_RESTART},
			idx:               0,
			expectResult:      []string{"start", "done", "start"},
			externalChan:      make(chan string, 10),
			externalEventChan: make(chan StatusEvent_t, 10),
		},
		{
			name:              "stop & start task",
			operation:         []TaskOperation_t{OPERATION_STOP, OPERATION_START},
			idx:               0,
			expectResult:      []string{"start", "done", "start"},
			externalChan:      make(chan string, 10),
			externalEventChan: make(chan StatusEvent_t, 10),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tf := func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
				tt.externalChan <- "start"
				for {
					select {
					case <-ctx.Done():
						tt.externalChan <- "done"
						return nil
					case event := <-tt.externalEventChan:
						eventChan <- event
					}
				}
			}

			msgRecord := make([]string, 0)
			isTestError := false

			defer func() {
				if isTestError {
					for _, msg := range msgRecord {
						fmt.Printf("msg: %v\n", msg)
					}
				}
			}()

			tm := NewTaskManager()
			tm.AddTask(tt.name, tf)
			tm.RunTask(tt.name)

			go func() {
				for {
					select {
					case msg := <-tm.GetEventMsg():
						msgRecord = append(msgRecord, msg.Msg)
					}
				}
			}()

			for _, operation := range tt.operation {
				tt.externalEventChan <- StatusEvent_t{
					Status:    nil,
					Operation: operation,
					Func:      nil,
				}
			}

			timeout := time.NewTimer(5 * time.Second)
		RESULT_LOOP:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task status timeout")
					isTestError = true
					return
				case result := <-tt.externalChan:

					if result != tt.expectResult[tt.idx] {
						t.Errorf("expect %v, but got %v", tt.expectResult[tt.idx], result)
						isTestError = true
					}

					tt.idx++
					if tt.idx == len(tt.expectResult) {
						break RESULT_LOOP
					}
				}
			}

			tm.StopTaskManager(true)
		})
	}
}

func TestTaskManager_Delete(t *testing.T) {
	tests := []struct {
		name              string
		operation         []TaskOperation_t
		idx               int
		expectResult      []string
		externalChan      chan string
		externalEventChan chan StatusEvent_t
		expectedExist     bool
	}{
		{
			name:              "delete without stop task",
			operation:         []TaskOperation_t{OPERATION_DELETE},
			idx:               0,
			expectResult:      []string{"start", "done"},
			externalChan:      make(chan string, 10),
			externalEventChan: make(chan StatusEvent_t, 10),
			expectedExist:     false,
		},
		{
			name:              "delete with stop task",
			operation:         []TaskOperation_t{OPERATION_STOP, OPERATION_DELETE},
			idx:               0,
			expectResult:      []string{"start", "done"},
			externalChan:      make(chan string, 10),
			externalEventChan: make(chan StatusEvent_t, 10),
			expectedExist:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgRecord := make([]string, 0)
			isTestError := false

			defer func() {
				if isTestError {
					for _, msg := range msgRecord {
						fmt.Printf("msg: %v\n", msg)
					}
				}
			}()

			tf := func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
				tt.externalChan <- "start"
				for {
					select {
					case <-ctx.Done():
						tt.externalChan <- "done"
						return nil
					case event := <-tt.externalEventChan:
						eventChan <- event
					}
				}
			}

			tm := NewTaskManager()
			tm.AddTask(tt.name, tf)
			tm.RunTask(tt.name)

			go func() {
				for {
					select {
					case msg := <-tm.GetEventMsg():
						msgRecord = append(msgRecord, msg.Msg)
					}
				}
			}()

			timeout := time.NewTimer(5 * time.Second)

			for _, operation := range tt.operation {
				tt.externalEventChan <- StatusEvent_t{
					Status:    nil,
					Operation: operation,
					Func:      nil,
				}
			}

		RESULT_LOOP:
			for {
				select {
				case <-timeout.C:
					t.Errorf("wait task status timeout")
					isTestError = true
					return
				case result := <-tt.externalChan:

					if result != tt.expectResult[tt.idx] {
						t.Errorf("expect %v, but got %v", tt.expectResult[tt.idx], result)
						isTestError = true
					}

					tt.idx++
					if tt.idx == len(tt.expectResult) {
						break RESULT_LOOP
					}
				}
			}

			timeout.Reset(5 * time.Second)
		TASK_DELETE:
			for {
				select {
				case <-timeout.C:
					t.Errorf("expect task not exist, but exist")
					isTestError = true
					return
				default:
					if tm.IsTaskExist(tt.name) == tt.expectedExist {
						break TASK_DELETE
					}

					time.Sleep(1 * time.Millisecond)
				}
			}

			tm.StopTaskManager(true)
		})
	}
}

func TestTaskManager_MultipleTaskStatusChange(t *testing.T) {
	tests := []struct {
		name  string
		tasks []struct {
			name     string
			tfunc    TaskFunc
			patterns []struct {
				statusEvent    StatusEvent_t
				expectedStatus interface{}
			}
		}
	}{
		{
			name: "multiple task",
			tasks: []struct {
				name     string
				tfunc    TaskFunc
				patterns []struct {
					statusEvent    StatusEvent_t
					expectedStatus interface{}
				}
			}{},
		},
	}

	var patternCount int = 10
	var taskCount int = 10

	var externalEventChans []chan StatusEvent_t = make([]chan StatusEvent_t, taskCount)
	for i := range externalEventChans {
		externalEventChans[i] = make(chan StatusEvent_t, 10)
	}

	for _, tt := range tests {
		for taskIdx := 0; taskIdx < taskCount; taskIdx++ {
			tt.tasks = append(tt.tasks, struct {
				name     string
				tfunc    TaskFunc
				patterns []struct {
					statusEvent    StatusEvent_t
					expectedStatus interface{}
				}
			}{})
			tt.tasks[taskIdx].tfunc = nil
			tt.tasks[taskIdx].name = fmt.Sprintf("task%d", taskIdx)

			for patternIdx := 0; patternIdx < patternCount; patternIdx++ {
				tt.tasks[taskIdx].patterns = append(tt.tasks[taskIdx].patterns, struct {
					statusEvent    StatusEvent_t
					expectedStatus interface{}
				}{
					statusEvent: StatusEvent_t{
						Status: fmt.Sprintf("%s_status%d", tt.tasks[taskIdx].name, patternIdx),
						Func:   nil,
					},
					expectedStatus: fmt.Sprintf("%s_status%d", tt.tasks[taskIdx].name, patternIdx),
				})

			}

			if tt.tasks[taskIdx].tfunc == nil {
				tt.tasks[taskIdx].tfunc = func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
					for {
						select {
						case <-ctx.Done():
							return nil
						case event := <-externalEventChans[taskIdx]:
							eventChan <- event
						}
					}
				}
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			msgRecord := make([]string, 0)
			isTestError := false

			defer func() {
				if isTestError {
					for _, msg := range msgRecord {
						fmt.Printf("msg: %v\n", msg)
					}
				}
			}()

			tm := NewTaskManager()

			for taskIdx := 0; taskIdx < len(tt.tasks); taskIdx++ {
				tm.AddTask(tt.tasks[taskIdx].name, tt.tasks[taskIdx].tfunc)
				tm.RunTask(tt.tasks[taskIdx].name)
			}

			go func() {
				for {
					select {
					case msg := <-tm.GetEventMsg():
						msgRecord = append(msgRecord, msg.Msg)
					}
				}
			}()

			wg := sync.WaitGroup{}

			for taskIdx := 0; taskIdx < len(tt.tasks); taskIdx++ {
				wg.Add(1)
				go func(taskIdx int) {
					defer wg.Done()
					for patternIdx := 0; patternIdx < patternCount; patternIdx++ {
						externalEventChans[taskIdx] <- tt.tasks[taskIdx].patterns[patternIdx].statusEvent

						timeout := time.NewTimer(5 * time.Second)

					WAIT_TASK_STATUS:
						for {
							select {
							case <-timeout.C:
								t.Errorf("wait task status timeout")
								isTestError = true
								return
							default:
								if taskStatus, err := tm.GetTaskStatus(tt.tasks[taskIdx].name); err == nil && taskStatus == tt.tasks[taskIdx].patterns[patternIdx].expectedStatus {
									break WAIT_TASK_STATUS
								} else {
									time.Sleep(1 * time.Millisecond)
								}
							}
						}
					}
				}(taskIdx)
			}

			wg.Wait()

			tm.StopTaskManager(true)
		})
	}
}

func TestTaskManager_MultipleTaskOperationChange(t *testing.T) {

	testPattern := []struct {
		operation    []TaskOperation_t
		expectResult []string
	}{
		{
			operation:    []TaskOperation_t{OPERATION_STOP},
			expectResult: []string{"start", "done"},
		},
		{
			operation:    []TaskOperation_t{OPERATION_RESTART},
			expectResult: []string{"start", "done", "start"},
		},
		{
			operation:    []TaskOperation_t{OPERATION_STOP, OPERATION_START},
			expectResult: []string{"start", "done", "start"},
		},
	}

	tests := []struct {
		name  string
		tasks []struct {
			name        string
			tfunc       TaskFunc
			patternsIdx int
		}
	}{
		{
			name: "multiple task",
			tasks: []struct {
				name        string
				tfunc       TaskFunc
				patternsIdx int
			}{},
		},
	}

	var taskCount int = 10

	var externalEventChans []chan StatusEvent_t = make([]chan StatusEvent_t, taskCount)
	for i := range externalEventChans {
		externalEventChans[i] = make(chan StatusEvent_t, 10)
	}

	var externalChan []chan string = make([]chan string, taskCount)
	for i := range externalChan {
		externalChan[i] = make(chan string, 10)
	}

	for _, tt := range tests {
		for taskIdx := 0; taskIdx < taskCount; taskIdx++ {
			tt.tasks = append(tt.tasks, struct {
				name        string
				tfunc       TaskFunc
				patternsIdx int
			}{})
			tt.tasks[taskIdx].name = fmt.Sprintf("task%d", taskIdx)
			tt.tasks[taskIdx].patternsIdx = taskIdx % len(testPattern)
			tt.tasks[taskIdx].tfunc = func(ctx context.Context, eventChan chan<- StatusEvent_t) error {
				externalChan[taskIdx] <- "start"
				for {
					select {
					case <-ctx.Done():
						externalChan[taskIdx] <- "done"
						return nil
					case event := <-externalEventChans[taskIdx]:
						eventChan <- event
					}
				}
			}
		}

		t.Run(tt.name, func(t *testing.T) {
			msgRecord := make([]string, 0)
			isTestError := false

			defer func() {
				if isTestError {
					for _, msg := range msgRecord {
						fmt.Printf("msg: %v\n", msg)
					}
				}
			}()

			tm := NewTaskManager()

			for taskIdx := 0; taskIdx < len(tt.tasks); taskIdx++ {
				tm.AddTask(tt.tasks[taskIdx].name, tt.tasks[taskIdx].tfunc)
				tm.RunTask(tt.tasks[taskIdx].name)
			}

			go func() {
				for {
					select {
					case msg := <-tm.GetEventMsg():
						msgRecord = append(msgRecord, msg.Msg)
					}
				}
			}()

			wg := sync.WaitGroup{}

			for taskIdx := 0; taskIdx < len(tt.tasks); taskIdx++ {
				wg.Add(1)
				go func(taskIdx int) {
					defer wg.Done()
					expectResultIdx := 0
					timeout := time.NewTimer(5 * time.Second)

					for _, operation := range testPattern[tt.tasks[taskIdx].patternsIdx].operation {
						externalEventChans[taskIdx] <- StatusEvent_t{
							Status:    nil,
							Operation: operation,
							Func:      nil,
						}
					}

				RESULT_LOOP:
					for {
						select {
						case <-timeout.C:
							t.Errorf("wait task status timeout")
							isTestError = true
							return
						case result := <-externalChan[taskIdx]:
							if result != testPattern[tt.tasks[taskIdx].patternsIdx].expectResult[expectResultIdx] {
								t.Errorf("expect %v, but got %v", testPattern[tt.tasks[taskIdx].patternsIdx].expectResult[expectResultIdx], result)
								isTestError = true
							}

							expectResultIdx++

							if expectResultIdx == len(testPattern[tt.tasks[taskIdx].patternsIdx].expectResult) {
								break RESULT_LOOP
							}

							timeout.Reset(5 * time.Second)
						}
					}
				}(taskIdx)
			}

			wg.Wait()

			tm.StopTaskManager(true)
		})
	}
}
