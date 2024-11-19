package taskmanager

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"
)

type TaskOperation_t int

const (
	OPERATION_NONE TaskOperation_t = iota
	OPERATION_START
	OPERATION_STOP
	OPERATION_RESTART
	OPERATION_DELETE
)

var operationStr = map[TaskOperation_t]string{
	OPERATION_NONE:    "none",
	OPERATION_START:   "start",
	OPERATION_STOP:    "stop",
	OPERATION_RESTART: "restart",
	OPERATION_DELETE:  "delete",
}

type StatusEvent_t struct {
	Status    interface{}
	Operation TaskOperation_t
	Func      TaskFunc
}

type TaskFunc func(ctx context.Context, eventChan chan<- StatusEvent_t) error

type taskInfo_t struct {
	taskName  string
	taskFunc  TaskFunc
	status    interface{}
	operation TaskOperation_t

	lastUpdate time.Time

	ctx    context.Context
	cancel context.CancelFunc

	// --- function ---
	funcCtx       context.Context
	funcCancel    context.CancelFunc
	isFuncRunning bool
	funcMutex     sync.Mutex
	funcIsLocked  bool
	funcResult    error

	eventChan chan StatusEvent_t

	parent *TaskManager_t
}

func (task *taskInfo_t) EventChange() {
	for {
		select {
		case <-task.ctx.Done():
			return
		case event := <-task.eventChan:
			if !reflect.DeepEqual(task.status, event.Status) {
				task.parent.sendMsg(fmt.Sprintf("Task %s status changed from %v to %v", task.taskName, task.status, event.Status), nil)
				task.status = event.Status
				task.lastUpdate = time.Now()
			}

			if event.Func != nil {
				task.parent.sendMsg(fmt.Sprintf("Task %s function changed", task.taskName), nil)
				task.taskFunc = event.Func
				task.lastUpdate = time.Now()
			}

			if event.Operation != OPERATION_NONE {
				task.parent.eventReceiver <- statusEventWithTask_t{
					operation: event.Operation,
					task:      task,
				}
			}
		}
	}
}

func (task *taskInfo_t) runFunc() {
	task.funcMutex.Lock()
	task.funcIsLocked = true

	task.funcResult = task.taskFunc(task.funcCtx, task.eventChan)

	task.funcMutex.Unlock()
	task.funcIsLocked = false
}
