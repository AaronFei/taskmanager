package taskmanager

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

type statusEventWithTask_t struct {
	operation TaskOperation_t
	task      *taskInfo_t
}

type TaskManagerMsg_t struct {
	Msg string
	Err error
	T   time.Time
}

type TaskManager_t struct {
	tasks map[string]*taskInfo_t

	ctx    context.Context
	cancel context.CancelFunc

	// --- Channel ---
	taskReceiver  chan *taskInfo_t
	eventReceiver chan statusEventWithTask_t

	requestTask chan struct {
		all  bool
		name string
	}
	reponseTask chan []*taskInfo_t

	msgNotify chan TaskManagerMsg_t
}

func NewTaskManager() *TaskManager_t {
	ctx, cancel := context.WithCancel(context.Background())
	taskManager := TaskManager_t{
		tasks:  make(map[string]*taskInfo_t),
		ctx:    ctx,
		cancel: cancel,

		taskReceiver:  make(chan *taskInfo_t, 256),
		eventReceiver: make(chan statusEventWithTask_t, 256),

		requestTask: make(chan struct {
			all  bool
			name string
		}, 10),
		reponseTask: make(chan []*taskInfo_t, 10),

		msgNotify: make(chan TaskManagerMsg_t, 256),
	}

	go taskManager.Service()

	return &taskManager
}

func (t *TaskManager_t) Service() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case taskName := <-t.requestTask:
			taskList := []*taskInfo_t{}

			if taskName.all {
				for _, v := range t.tasks {
					taskList = append(taskList, v)
				}
			} else {
				if task, ok := t.tasks[taskName.name]; ok {
					taskList = append(taskList, task)
				}
			}

			t.reponseTask <- taskList
		case task := <-t.taskReceiver:
			t.sendMsg(fmt.Sprintf("task %s added", task.taskName), nil)
			t.tasks[task.taskName] = task

		case event := <-t.eventReceiver:
			if _, ok := t.tasks[event.task.taskName]; !ok {
				t.sendMsg(fmt.Sprintf("task %s not found", event.task.taskName), nil)
				continue
			}

			t.sendMsg(fmt.Sprintf("task %s operation changed to %s", event.task.taskName, operationStr[event.operation]), nil)
			switch event.operation {
			case OPERATION_START:
				if event.task.taskFunc != nil && !event.task.isFuncRunning {
					funcCtx, funcCancel := context.WithCancel(event.task.ctx)
					event.task.funcCtx = funcCtx
					event.task.funcCancel = funcCancel
					go event.task.runFunc()
					event.task.isFuncRunning = true
				} else if event.task.isFuncRunning {
					t.sendMsg(fmt.Sprintf("task %s is already running", event.task.taskName), nil)
				}
			case OPERATION_STOP:
				if event.task.funcCancel != nil && event.task.isFuncRunning {
					event.task.funcCancel()
					event.task.funcCancel = nil
					event.task.isFuncRunning = false
				} else {
					t.sendMsg(fmt.Sprintf("task %s is not running", event.task.taskName), nil)
				}
			case OPERATION_DELETE:
				if event.task.cancel != nil {
					event.task.cancel()
					event.task.cancel = nil
				}
				delete(t.tasks, event.task.taskName)
			case OPERATION_RESTART:
				t.eventReceiver <- statusEventWithTask_t{
					operation: OPERATION_STOP,
					task:      event.task,
				}

				t.eventReceiver <- statusEventWithTask_t{
					operation: OPERATION_START,
					task:      event.task,
				}
			}

			event.task.operation = event.operation
			event.task.lastUpdate = time.Now()
		}
	}
}

func (t *TaskManager_t) AddTask(taskName string, taskFunc TaskFunc) {
	ctx, cancel := context.WithCancel(t.ctx)

	taskInfo := taskInfo_t{
		taskName:   taskName,
		taskFunc:   taskFunc,
		status:     nil,
		operation:  OPERATION_NONE,
		lastUpdate: time.Time{},

		ctx:    ctx,
		cancel: cancel,

		eventChan: make(chan StatusEvent_t, 10),
		parent:    t,

		funcCtx:       nil,
		funcCancel:    nil,
		isFuncRunning: false,
		funcMutex:     sync.Mutex{},
		funcResult:    nil,
	}

	go taskInfo.EventChange()

	t.taskReceiver <- &taskInfo

	for {
		if t.IsTaskExist(taskName) {
			break
		}
	}
}

func getTaskInfoFromRequstCh(t *TaskManager_t, taskName string, all bool) ([]*taskInfo_t, error) {
	t.requestTask <- struct {
		all  bool
		name string
	}{name: taskName, all: all}

	response := <-t.reponseTask

	if len(response) == 0 {
		return nil, fmt.Errorf("task not found")
	}

	return response, nil
}

func changeOperation(t *TaskManager_t, taskName string, operation TaskOperation_t) error {
	tasks, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return err
	}

	t.eventReceiver <- statusEventWithTask_t{
		operation: operation,
		task:      tasks[0],
	}

	return nil
}

func (t *TaskManager_t) DeleteTask(taskName string) error {
	return changeOperation(t, taskName, OPERATION_DELETE)
}

func (t *TaskManager_t) DeleteAllTask() error {
	taskInfo, err := getTaskInfoFromRequstCh(t, "", true)
	if err != nil {
		return err
	}

	err = nil
	for _, v := range taskInfo {
		err = t.DeleteTask(v.taskName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *TaskManager_t) RunTask(taskName string) error {
	return changeOperation(t, taskName, OPERATION_START)
}

func (t *TaskManager_t) StopTask(taskName string) error {
	return changeOperation(t, taskName, OPERATION_STOP)
}

func (t *TaskManager_t) StopAllTask() error {
	taskInfo, err := getTaskInfoFromRequstCh(t, "", true)
	if err != nil {
		return err
	}

	err = nil
	for _, v := range taskInfo {
		err = t.StopTask(v.taskName)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *TaskManager_t) RestartTask(taskName string) error {
	return changeOperation(t, taskName, OPERATION_RESTART)
}

func (t *TaskManager_t) CloseTaskManager(waitMsg bool) {
	t.sendMsg("TaskManager close", nil)
	t.DeleteAllTask()
	t.cancel()

	if waitMsg {
		for {
			if len(t.msgNotify) == 0 {
				break
			}

			time.Sleep(1 * time.Millisecond)
		}
	}

	close(t.msgNotify)
	close(t.taskReceiver)
	close(t.eventReceiver)
	close(t.requestTask)
	close(t.reponseTask)
}

func (t *TaskManager_t) GetTaskStatus(taskName string) (interface{}, error) {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return nil, err
	}

	return taskInfo[0].status, nil
}

func (t *TaskManager_t) GetTaskOperation(taskName string) (TaskOperation_t, error) {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return OPERATION_NONE, err
	}

	return taskInfo[0].operation, nil
}

func (t *TaskManager_t) GetTaskLastUpdateTime(taskName string) (time.Time, error) {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return time.Time{}, err
	}

	return taskInfo[0].lastUpdate, nil
}

func (t *TaskManager_t) GetTaskFuncResult(taskName string) error {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return err
	}

	return taskInfo[0].funcResult
}

func (t *TaskManager_t) GetTaskNameList() ([]string, error) {
	taskInfo, err := getTaskInfoFromRequstCh(t, "", true)
	if err != nil {
		return nil, err
	}

	taskNameList := []string{}
	for _, v := range taskInfo {
		taskNameList = append(taskNameList, v.taskName)
	}

	return taskNameList, nil
}

func (t *TaskManager_t) GetTaskFuncLockStatus(taskName string) bool {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return false
	}

	return taskInfo[0].funcIsLocked
}

func (t *TaskManager_t) GetTotalTaskCount() int {
	tasks, err := getTaskInfoFromRequstCh(t, "", true)

	if err != nil {
		return 0
	}

	return len(tasks)
}

func (t *TaskManager_t) GetTotalTaskCountByStatus(status interface{}) (int, error) {
	taskInfo, err := getTaskInfoFromRequstCh(t, "", true)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, v := range taskInfo {
		if reflect.DeepEqual(v.status, status) {
			count++
		}
	}

	return count, nil
}

func (t *TaskManager_t) IsTaskExist(taskName string) bool {
	taskInfo, err := getTaskInfoFromRequstCh(t, taskName, false)
	if err != nil {
		return false
	}

	return taskInfo[0] != nil
}

func (t *TaskManager_t) sendMsg(msg string, err error) {
	select {
	case t.msgNotify <- TaskManagerMsg_t{
		Msg: msg,
		Err: err,
		T:   time.Now(),
	}:
	default:
	}
}

func (t *TaskManager_t) GetEventMsg() <-chan TaskManagerMsg_t {
	return t.msgNotify
}
