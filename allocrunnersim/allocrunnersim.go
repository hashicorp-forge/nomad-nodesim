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
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/device"
	"github.com/hashicorp/nomad/plugins/drivers"
	"golang.org/x/exp/maps"
)

const (
	// taskEventMessagePrefix should be used as a prefix to all task event
	// messages. It helps consistency and makes it clear which events are
	// coming from the simulated runner.
	taskEventMessagePrefix = "A nodesim task event happened: "
)

type simulatedAllocRunner struct {
	c      cinterfaces.AllocStateHandler
	logger hclog.Logger
	id     string

	alloc     *structs.Allocation
	allocLock sync.RWMutex

	allocState     *state.State
	allocStateLock sync.RWMutex

	// lastAcknowledgedState is the alloc runner state that was last
	// acknowledged by the server. It may lag behind allocState.
	lastAcknowledgedState *state.State
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

	// Pull the tasks from the alloc-runner, so we only have to do this once.
	allocRunnerTasks := ar.Alloc().Job.LookupTaskGroup(ar.alloc.TaskGroup).Tasks

	// Track the tasks states directly within this function. This is fine as
	// there is no outside influence of them such as an actual task runner.
	taskStates := make(map[string]*structs.TaskState, len(allocRunnerTasks))

	// This point marks the point where we add task events and eventually mark
	// them as started.
	for _, task := range allocRunnerTasks {

		taskStates[task.Name] = structs.NewTaskState()

		event := &structs.TaskEvent{
			Type:    structs.TaskSetup,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskSetup,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)
	}
	ar.updateAllocAndSendUpdate(taskStates)

	// Let us pretend we are building a task directory, which is a fairly
	// common event to see.
	time.Sleep(100 * time.Millisecond)
	ar.logger.Debug("building task directory")

	for _, task := range allocRunnerTasks {
		event := &structs.TaskEvent{
			Type:    structs.TaskBuildingTaskDir,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskBuildingTaskDir,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)
	}
	ar.updateAllocAndSendUpdate(taskStates)

	// Trigger a task hook which is normal operation when starting a typical
	// Nomad task.
	time.Sleep(200 * time.Millisecond)
	ar.logger.Debug("firing a task hook")

	for _, task := range allocRunnerTasks {
		event := &structs.TaskEvent{
			Type:    structs.TaskHookMessage,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskHookMessage,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)
	}
	ar.updateAllocAndSendUpdate(taskStates)

	// Trigger another task hook, which is normal operation when starting a
	// typical Nomad task.
	time.Sleep(100 * time.Millisecond)
	ar.logger.Debug("firing another task hook")

	for _, task := range allocRunnerTasks {
		event := &structs.TaskEvent{
			Type:    structs.TaskHookMessage,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskHookMessage,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)
	}
	ar.updateAllocAndSendUpdate(taskStates)

	// The tasks are now started. Add a task event to detail this and also
	// modify the state to indicate this and when it happened.
	time.Sleep(500 * time.Millisecond)
	ar.logger.Debug("marking all tasks as started")

	for _, task := range allocRunnerTasks {

		event := &structs.TaskEvent{
			Type:    structs.TaskStarted,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskStarted,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)

		taskStates[task.Name].StartedAt = time.Now()
		taskStates[task.Name].State = structs.TaskStateRunning
	}

	// Mark the deployment as healthy by directly manipulating the state
	// object. This ensures the Nomad CLI will return when polling the job
	// registration deployment.
	time.Sleep(100 * time.Millisecond)
	ar.logger.Debug("marking deployment as healthy")

	ar.allocStateLock.Lock()
	ar.allocState.DeploymentStatus = &structs.AllocDeploymentStatus{Healthy: pointer.Of(true), Timestamp: time.Now()}
	ar.allocStateLock.Unlock()

	ar.updateAllocAndSendUpdate(taskStates)

	// Who wants to live forever?
	//
	// Batch and sysbatch jobs certainly don't, so after a little pause to
	// simulate the allocation has done something, we stop it. This is useful
	// for load testing, as we can continually dispatch jobs, without nodes
	// becoming resource exhausted and still use the lighter-weight simulated
	// alloc-runner.
	//
	// Other job types which are meant to run forever, must be stopped by a
	// user initiated command.
	switch ar.alloc.Job.Type {
	case structs.JobTypeBatch, structs.JobTypeSysBatch:
		go func() {
			time.Sleep(5 * time.Second)
			ar.stopAll()
		}()
	}
}

// updateAllocAndSendUpdate is a small helper that builds a new allocation
// object from the potentially updated task states. This function should be
// called whenever a change has happened that you want sent to the servers. The
// change might not be immediately sent and is calculated according to
// GetUpdatePriority.
func (ar *simulatedAllocRunner) updateAllocAndSendUpdate(taskStates map[string]*structs.TaskState) {
	hydratedAlloc := ar.clientAlloc(taskStates)
	ar.c.AllocStateUpdated(hydratedAlloc)
}

// clientAlloc is lifted from Nomad and populates a new allocation based on the
// current alloc state.
func (ar *simulatedAllocRunner) clientAlloc(taskStates map[string]*structs.TaskState) *structs.Allocation {
	ar.allocStateLock.Lock()
	defer ar.allocStateLock.Unlock()

	// store task states for AllocState to expose
	ar.allocState.TaskStates = taskStates

	a := &structs.Allocation{
		ID:         ar.id,
		TaskStates: taskStates,
	}

	if d := ar.allocState.DeploymentStatus; d != nil {
		a.DeploymentStatus = d.Copy()
	}

	// Compute the ClientStatus
	if ar.allocState.ClientStatus != "" {
		// The client status is being forced
		a.ClientStatus, a.ClientDescription = ar.allocState.ClientStatus, ar.allocState.ClientDescription
	} else {
		a.ClientStatus, a.ClientDescription = getClientStatus(taskStates)
	}

	// If the allocation is terminal, make sure all required fields are properly
	// set.
	if a.ClientTerminalStatus() {
		alloc := ar.Alloc()

		// If we are part of a deployment and the alloc has failed, mark the
		// alloc as unhealthy. This guards against the watcher not be started.
		// If the health status is already set then terminal allocations should not
		if a.ClientStatus == structs.AllocClientStatusFailed &&
			alloc.DeploymentID != "" && !a.DeploymentStatus.HasHealth() {
			a.DeploymentStatus = &structs.AllocDeploymentStatus{
				Healthy: pointer.Of(false),
			}
		}

		// Make sure we have marked the finished at for every task. This is used
		// to calculate the reschedule time for failed allocations.
		now := time.Now()
		for _, taskName := range ar.taskNamesLocked() {
			ts, ok := a.TaskStates[taskName]
			if !ok {
				ts = &structs.TaskState{}
				a.TaskStates[taskName] = ts
			}
			if ts.FinishedAt.IsZero() {
				ts.FinishedAt = now
			}
		}
	}

	// Set the NetworkStatus and default DNSConfig if one is not returned from the client
	netStatus := ar.allocState.NetworkStatus
	if netStatus != nil {
		a.NetworkStatus = netStatus
	} else {
		a.NetworkStatus = new(structs.AllocNetworkStatus)
	}

	if a.NetworkStatus.DNS == nil {
		alloc := ar.Alloc()
		nws := alloc.Job.LookupTaskGroup(alloc.TaskGroup).Networks
		if len(nws) > 0 {
			a.NetworkStatus.DNS = nws[0].DNS.Copy()
		}
	}

	return a
}

// getClientStatus takes in the task states for a given allocation and computes
// the client status and description.
func getClientStatus(taskStates map[string]*structs.TaskState) (status, description string) {
	var pending, running, dead, failed bool
	for _, taskState := range taskStates {
		switch taskState.State {
		case structs.TaskStateRunning:
			running = true
		case structs.TaskStatePending:
			pending = true
		case structs.TaskStateDead:
			if taskState.Failed {
				failed = true
			} else {
				dead = true
			}
		}
	}

	// Determine the alloc status
	if failed {
		return structs.AllocClientStatusFailed, "Failed tasks"
	} else if running {
		return structs.AllocClientStatusRunning, "Tasks are running"
	} else if pending {
		return structs.AllocClientStatusPending, "No tasks have started"
	} else if dead {
		return structs.AllocClientStatusComplete, "All tasks have completed"
	}

	return "", ""
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

	// Get the current list of task being run by the alloc-runner, so we can
	// iterate this as many times as needed.
	ar.allocLock.RLock()
	allocRunnerTasks := ar.Alloc().Job.LookupTaskGroup(ar.alloc.TaskGroup).Tasks
	ar.allocLock.RUnlock()

	// Ensure we have a current copy of the task states, so that we append and
	// create a correct and full list.
	ar.allocStateLock.RLock()
	taskStates := ar.allocState.TaskStates
	ar.allocStateLock.RUnlock()

	// Perform the task kill, which essentially is the shutdown notification.
	ar.logger.Debug("performing task kill")

	for _, task := range allocRunnerTasks {
		event := &structs.TaskEvent{
			Type:    structs.TaskKilling,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskKilling,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)
	}
	ar.updateAllocAndSendUpdate(taskStates)

	// Mark the tasks as killed and ensure the state is updated to show they
	// are dead and finished.
	time.Sleep(200 * time.Millisecond)
	ar.logger.Debug("marking task as killed")

	for _, task := range allocRunnerTasks {
		event := &structs.TaskEvent{
			Type:    structs.TaskKilled,
			Time:    time.Now().UnixNano(),
			Message: taskEventMessagePrefix + structs.TaskKilled,
		}
		event.PopulateEventDisplayMessage()
		taskStates[task.Name].Events = append(taskStates[task.Name].Events, event)

		ar.allocState.TaskStates[task.Name].FinishedAt = time.Now()
		ar.allocState.TaskStates[task.Name].State = structs.TaskStateDead
	}
	ar.updateAllocAndSendUpdate(taskStates)

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
	ar.allocStateLock.RLock()
	defer ar.allocStateLock.RUnlock()
	return ar.allocState.Copy()
}

func (ar *simulatedAllocRunner) PersistState() error { return nil }

func (ar *simulatedAllocRunner) AcknowledgeState(allocState *state.State) {
	ar.allocStateLock.Lock()
	defer ar.allocStateLock.Unlock()
	ar.lastAcknowledgedState = allocState
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
	ar.allocStateLock.RLock()
	defer ar.allocStateLock.RUnlock()

	last := ar.lastAcknowledgedState
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
