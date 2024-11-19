# Go Task Manager

A lightweight and flexible task management library for Go applications that provides concurrent task execution with status monitoring and operation control.

## Features

- 🔄 Concurrent task execution and management
- 📊 Real-time task status monitoring
- 🎮 Dynamic task control (start/stop/restart)
- 🔍 Task status tracking and updates
- 📝 Event-based messaging system
- 🔒 Thread-safe operations
- 🎯 Context-based cancellation support

## Installation

```bash
go get github.com/yourusername/taskmanager
```

## Usage

### Basic Example

```go
package main

import (
    "context"
    "fmt"
    "time"
    "github.com/yourusername/taskmanager"
)

func main() {
    // Create a new task manager
    tm := taskmanager.NewTaskManager()
    defer tm.StopTaskManager(true)

    // Define a sample task
    sampleTask := func(ctx context.Context, eventChan chan<- taskmanager.StatusEvent_t) error {
        count := 0
        for {
            select {
            case <-ctx.Done():
                return nil
            default:
                count++
                eventChan <- taskmanager.StatusEvent_t{
                    Status: count,
                }
                time.Sleep(time.Second)
            }
        }
    }

    // Add and run the task
    tm.AddTask("counter", sampleTask)
    tm.RunTask("counter")

    // Monitor task messages
    go func() {
        for msg := range tm.GetEventMsg() {
            fmt.Printf("Message: %s, Time: %s\n", msg.Msg, msg.T)
        }
    }()

    // Let it run for a while
    time.Sleep(5 * time.Second)
}
```

### Task Operations

```go
// Start a task
tm.RunTask("taskName")

// Stop a task
tm.StopTask("taskName")

// Restart a task
tm.RestartTask("taskName")

// Delete a task
tm.DeleteTask("taskName")

// Stop all tasks
tm.StopAllTask()
```

### Task Status Monitoring

```go
// Get task status
status, err := tm.GetTaskStatus("taskName")

// Get task operation state
op, err := tm.GetTaskOperation("taskName")

// Get last update time
lastUpdate, err := tm.GetTaskLastUpdateTime("taskName")

// Get task function result
result := tm.GetTaskFuncResult("taskName")

// Get list of all task names
taskNames, err := tm.GetTaskNameList()

// Get total task count
count := tm.GetTotalTaskCount()
```

## Task Status Events

Tasks can emit status events to update their state:

```go
eventChan <- taskmanager.StatusEvent_t{
    Status:    newStatus,    // Any type of status data
    Operation: taskmanager.OPERATION_NONE,
    Func:      nil,         // Optional new task function
}
```

## Message Monitoring

The task manager provides a message channel for monitoring system events:

```go
for msg := range tm.GetEventMsg() {
    fmt.Printf("Message: %s\n", msg.Msg)
    if msg.Err != nil {
        fmt.Printf("Error: %v\n", msg.Err)
    }
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
