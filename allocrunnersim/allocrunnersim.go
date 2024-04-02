// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package allocrunnersim

import (
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client/allocdir"
	"github.com/hashicorp/nomad/client/allocrunner/interfaces"
	"github.com/hashicorp/nomad/client/allocrunner/state"
	"github.com/hashicorp/nomad/client/config"
	cinterfaces "github.com/hashicorp/nomad/client/interfaces"
	"github.com/hashicorp/nomad/client/pluginmanager/drivermanager"
	cstructs "github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/device"
	"github.com/hashicorp/nomad/plugins/drivers"
	"golang.org/x/exp/maps"
)

type simulatedAllocRunner struct {
	c          cinterfaces.AllocStateHandler
	logger     hclog.Logger
	id         string
	alloc      *structs.Allocation
	allocState *state.State
	allocLock  sync.RWMutex
}

func NewEmptyAllocRunnerFunc(conf *config.AllocRunnerConfig) (interfaces.AllocRunner, error) {
	return &simulatedAllocRunner{
		c:          conf.StateUpdater,
		logger:     conf.Logger.With("alloc_id", conf.Alloc.ID, "job_id", conf.Alloc.JobID, "namespace", conf.Alloc.Namespace),
		id:         conf.Alloc.ID,
		alloc:      conf.Alloc,
		allocState: &state.State{},
	}, nil
}

func (ar *simulatedAllocRunner) Alloc() *structs.Allocation {
	ar.allocLock.RLock()
	defer ar.allocLock.RUnlock()
	return ar.alloc.Copy()
}

func (ar *simulatedAllocRunner) taskNamesLocked() []string {
	taskNames := []string{}
	tg := ar.alloc.Job.LookupTaskGroup(ar.alloc.TaskGroup)
	for _, task := range tg.Tasks {
		taskNames = append(taskNames, task.Name)
	}
	return taskNames
}

func (ar *simulatedAllocRunner) Run() {
	ar.logger.Info("running allocation")

	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.allocState.TaskStates = map[string]*structs.TaskState{}
		for _, task := range ar.taskNamesLocked() {
			ar.allocState.TaskStates[task] = structs.NewTaskState()
			ar.allocState.TaskStates[task].StartedAt = time.Now()
			ar.allocState.TaskStates[task].State = structs.TaskStateRunning
		}

		ar.appendTaskEventForLocked(structs.TaskSetup)
		ar.alloc.TaskStates = ar.allocState.TaskStates
	})

	time.Sleep(100 * time.Millisecond)
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.logger.Debug("building taskdir", "alloc_id", ar.id)
		ar.appendTaskEventForLocked(structs.TaskBuildingTaskDir)
	})

	time.Sleep(200 * time.Millisecond)
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.logger.Debug("firing a task hook", "alloc_id", ar.id)
		ar.appendTaskEventForLocked(structs.TaskHookMessage)
	})

	time.Sleep(100 * time.Millisecond)
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.logger.Debug("firing another task hook", "alloc_id", ar.id)
		ar.appendTaskEventForLocked(structs.TaskHookMessage)
	})

	time.Sleep(500 * time.Millisecond)
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.logger.Debug("tasks are started", "alloc_id", ar.id)
		ar.appendTaskEventForLocked(structs.TaskStarted)
		ar.allocState.ClientStatus = "running"
		ar.alloc.ClientStatus = "running"
		ar.allocState.SetDeploymentStatus(time.Now(), true)
		ar.alloc.DeploymentStatus = ar.allocState.DeploymentStatus.Copy()
	})

}

// updateAllocAndSendUpdate is a helper that updates the allocrunner state while
// the allocLock is held, and then queues-up a server update
func (ar *simulatedAllocRunner) updateAllocAndSendUpdate(updateFn func(*simulatedAllocRunner)) {
	ar.allocLock.Lock()
	updateFn(ar)
	ar.c.AllocStateUpdated(ar.alloc)
	ar.allocLock.Unlock()
}

func (ar *simulatedAllocRunner) appendTaskEventForLocked(eventType string) {
	event := &structs.TaskEvent{
		Type:    eventType,
		Time:    time.Now().UnixNano(),
		Message: "a task event happened: " + eventType,
	}
	event.PopulateEventDisplayMessage()

	for _, task := range ar.taskNamesLocked() {
		ar.allocState.TaskStates[task].Events = append(ar.allocState.TaskStates[task].Events, event)
		ar.alloc.TaskStates = ar.allocState.TaskStates
	}
}

func (ar *simulatedAllocRunner) Restore() error { return nil }

func (ar *simulatedAllocRunner) Update(update *structs.Allocation) {

	// Be careful with the lock, as further down in we call functions that also
	// use the lock.
	ar.allocLock.Lock()
	ar.alloc = update
	ar.allocLock.Unlock()

	ar.logger.Info("received allocation update", "desired_status", update.DesiredStatus)

	switch update.DesiredStatus {
	case structs.AllocDesiredStatusStop:
		ar.stopAll()
	default:
		ar.logger.Warn("unable to handle desired status update", "desired_status", update.DesiredStatus)
	}
}

func (ar *simulatedAllocRunner) stopAll() {

	// Modify the task states, so that they report as "dead" meaning they are
	// terminal.
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		for _, task := range ar.taskNamesLocked() {
			ar.allocState.TaskStates[task].FinishedAt = time.Now()
			ar.allocState.TaskStates[task].State = structs.TaskStateDead
		}
		ar.alloc.TaskStates = ar.allocState.TaskStates
	})

	// Sleep a little, we don't want things to go too fast now, do we?
	time.Sleep(100 * time.Millisecond)

	// Update the overall allocation status to indicate the tasks have stopped
	// and the allocation, therefore, is complete.
	ar.updateAllocAndSendUpdate(func(ar *simulatedAllocRunner) {
		ar.logger.Debug("tasks are shutdown", "alloc_id", ar.id)
		ar.appendTaskEventForLocked(structs.TaskKilled)
		ar.allocState.ClientStatus = structs.AllocClientStatusComplete
		ar.alloc.ClientStatus = structs.AllocClientStatusComplete
		ar.alloc.ClientDescription = "All tasks have completed"
		ar.allocState.ClientDescription = "All tasks have completed"
	})

	ar.logger.Info("stopped all alloc-runner tasks and marked as complete")
}

func (ar *simulatedAllocRunner) Reconnect(update *structs.Allocation) error {
	ar.allocLock.Lock()
	defer ar.allocLock.Unlock()
	ar.alloc = update
	return nil
}

func (ar *simulatedAllocRunner) Shutdown() {}
func (ar *simulatedAllocRunner) Destroy()  {}

func (ar *simulatedAllocRunner) IsDestroyed() bool { return false }
func (ar *simulatedAllocRunner) IsMigrating() bool { return false }
func (ar *simulatedAllocRunner) IsWaiting() bool   { return false }

func (ar *simulatedAllocRunner) WaitCh() <-chan struct{} { return make(chan struct{}) }

func (ar *simulatedAllocRunner) DestroyCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (ar *simulatedAllocRunner) ShutdownCh() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (ar *simulatedAllocRunner) AllocState() *state.State {
	ar.allocLock.RLock()
	defer ar.allocLock.RUnlock()
	return ar.allocState.Copy()
}

func (ar *simulatedAllocRunner) PersistState() error { return nil }

func (ar *simulatedAllocRunner) AcknowledgeState(allocState *state.State) {
	ar.allocLock.Lock()
	defer ar.allocLock.Unlock()
	ar.allocState = allocState
}

func (ar *simulatedAllocRunner) SetClientStatus(status string) {
	ar.allocLock.Lock()
	defer ar.allocLock.Unlock()
	ar.alloc.ClientStatus = status
}

func (ar *simulatedAllocRunner) Signal(taskName, signal string) error { return nil }
func (ar *simulatedAllocRunner) RestartTask(taskName string, taskEvent *structs.TaskEvent) error {
	return nil
}
func (ar *simulatedAllocRunner) RestartRunning(taskEvent *structs.TaskEvent) error { return nil }
func (ar *simulatedAllocRunner) RestartAll(taskEvent *structs.TaskEvent) error     { return nil }

func (ar *simulatedAllocRunner) GetTaskEventHandler(taskName string) drivermanager.EventHandler {
	return nil
}
func (ar *simulatedAllocRunner) GetTaskExecHandler(taskName string) drivermanager.TaskExecHandler {
	return nil
}
func (ar *simulatedAllocRunner) GetTaskDriverCapabilities(taskName string) (*drivers.Capabilities, error) {
	return nil, nil
}

func (ar *simulatedAllocRunner) GetUpdatePriority(alloc *structs.Allocation) cstructs.AllocUpdatePriority {
	ar.allocLock.RLock()
	defer ar.allocLock.RUnlock()

	last := ar.allocState
	if last == nil {
		if alloc.ClientStatus == structs.AllocClientStatusFailed {
			return cstructs.AllocUpdatePriorityUrgent
		}
		return cstructs.AllocUpdatePriorityTypical
	}

	switch {
	case last.ClientStatus != alloc.ClientStatus:
		return cstructs.AllocUpdatePriorityUrgent
	case last.ClientDescription != alloc.ClientDescription:
		return cstructs.AllocUpdatePriorityTypical
	case !last.DeploymentStatus.Equal(alloc.DeploymentStatus):
		return cstructs.AllocUpdatePriorityTypical
	case !last.NetworkStatus.Equal(alloc.NetworkStatus):
		return cstructs.AllocUpdatePriorityTypical
	}

	if !maps.EqualFunc(last.TaskStates, alloc.TaskStates, func(st, o *structs.TaskState) bool {
		return st.Equal(o)

	}) {
		return cstructs.AllocUpdatePriorityTypical
	}

	return cstructs.AllocUpdatePriorityNone
}

func (ar *simulatedAllocRunner) StatsReporter() interfaces.AllocStatsReporter { return ar }
func (ar *simulatedAllocRunner) Listener() *cstructs.AllocListener            { return nil }
func (ar *simulatedAllocRunner) GetAllocDir() *allocdir.AllocDir              { return nil }

// LatestAllocStats lets this empty runner implement AllocStatsReporter
func (ar *simulatedAllocRunner) LatestAllocStats(taskFilter string) (*cstructs.AllocResourceUsage, error) {
	return &cstructs.AllocResourceUsage{
		ResourceUsage: &cstructs.ResourceUsage{
			MemoryStats: &cstructs.MemoryStats{},
			CpuStats:    &cstructs.CpuStats{},
			DeviceStats: []*device.DeviceGroupStats{},
		},
		Tasks:     map[string]*cstructs.TaskResourceUsage{},
		Timestamp: 0,
	}, nil
}
