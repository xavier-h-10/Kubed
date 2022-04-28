package pleg

import (
	"fmt"
	"minik8s/apiObject"
	"minik8s/kubelet/src/podutil"
	"minik8s/kubelet/src/runtime/container"
	"minik8s/kubelet/src/runtime/pod"
	"minik8s/kubelet/src/status"
	"minik8s/kubelet/src/types"
	"time"
)

const (
	eventChannelSize      = 10
	relistIntervalSeconds = 10
)

// PodLifecycleEventType define the event type of pod life cycle events.
type PodLifecycleEventType string

const (
	// ContainerStarted - event type when the new state of container is running.
	ContainerStarted PodLifecycleEventType = "ContainerStarted"
	// ContainerDied - event type when the new state of container is exited.
	ContainerDied PodLifecycleEventType = "ContainerDied"
	// ContainerRemoved - event type when the old state of container is exited.
	ContainerRemoved PodLifecycleEventType = "ContainerRemoved"
	// ContainerNeedStart - event type when the container is needed to start.
	ContainerNeedStart PodLifecycleEventType = "ContainerNeedStart"
	// ContainerNeedRestart - event type when the container needs to restart.
	ContainerNeedRestart PodLifecycleEventType = "ContainerNeedRestart"
	// ContainerNeedCreateAndStart - event type when the container needs to create and start.
	ContainerNeedCreateAndStart PodLifecycleEventType = "ContainerNeedCreateAndStart"
	// ContainerNeedRemove - event type when the container needs to be removed.
	ContainerNeedRemove PodLifecycleEventType = "ContainerNeedRemove"
	// PodSync is used to trigger syncing of a pod when the observed change of
	// the state of the pod cannot be captured by any single event above.
	PodSync PodLifecycleEventType = "PodSync"
	// ContainerChanged - event type when the new state of container is unknown.
	ContainerChanged PodLifecycleEventType = "ContainerChanged"
)

// PodLifecycleEvent is an event that reflects the change of the pod state.
type PodLifecycleEvent struct {
	// The pod ID.
	ID types.UID
	// The type of the event.
	Type PodLifecycleEventType
	// The accompanied data which varies based on the event type.
	//   - ContainerStarted/ContainerStopped: the container name (string).
	//   - All other event types: unused.
	Data interface{}
}

type podStatusRecord struct {
	OldStatus     *pod.PodStatus
	CurrentStatus *pod.PodStatus
}

type podStatusRecords map[types.UID]*podStatusRecord

func (statusRecords podStatusRecords) UpdateRecord(podUID types.UID, newStatus *pod.PodStatus) {
	if record, exists := statusRecords[podUID]; exists {
		record.OldStatus = record.CurrentStatus
		record.CurrentStatus = newStatus
	} else {
		statusRecords[podUID] = &podStatusRecord{
			OldStatus:     nil,
			CurrentStatus: newStatus,
		}
	}
}

func (statusRecords podStatusRecords) RemoveRecord(podUID types.UID) {
	delete(statusRecords, podUID)
}

func (statusRecords podStatusRecords) GetRecord(podUID types.UID) *podStatusRecord {
	return statusRecords[podUID]
}

type Manager interface {
	Updates() chan *PodLifecycleEvent
	Start()
}

func NewPlegManager(statusManager status.Manager, podManager pod.Manager) Manager {
	return &manager{
		eventCh:          make(chan *PodLifecycleEvent, eventChannelSize),
		statusManager:    statusManager,
		podManager:       podManager,
		podStatusRecords: make(podStatusRecords),
	}
}

type manager struct {
	eventCh          chan *PodLifecycleEvent
	statusManager    status.Manager
	podManager       pod.Manager
	podStatusRecords podStatusRecords
}

func newPodLifecycleEvent(podUID types.UID, eventType PodLifecycleEventType, containerID container.ContainerID) *PodLifecycleEvent {
	return &PodLifecycleEvent{
		ID:   podUID,
		Type: eventType,
		Data: containerID,
	}
}

func (m *manager) addStartedLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerStarted, containerID)
}

func (m *manager) addNeedRemoveLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerNeedRemove, containerID)
}

func (m *manager) addNeedRestartLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerNeedRestart, containerID)
}

func (m *manager) addNeedStartLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerNeedStart, containerID)
}

func (m *manager) addNeedCreateAndStartLifecycleEvent(podUID types.UID, containerName string) {
	m.eventCh <- &PodLifecycleEvent{
		ID:   podUID,
		Type: ContainerNeedCreateAndStart,
		Data: containerName,
	}
}

func (m *manager) addDiedLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerDied, containerID)
}

func (m *manager) addRemovedLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerRemoved, containerID)
}

func (m *manager) addPodSyncLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, PodSync, containerID)
}

func (m *manager) addChangedLifecycleEvent(podUID types.UID, containerID container.ContainerID) {
	m.eventCh <- newPodLifecycleEvent(podUID, ContainerChanged, containerID)
}

func (m *manager) removeAllContainers(runtimePodStatus *pod.PodStatus) {
	for _, cs := range runtimePodStatus.ContainerStatuses {
		m.addNeedRemoveLifecycleEvent(runtimePodStatus.ID, cs.ID)
	}
}

// compareAndProduceLifecycleEvents compares given runtime pod statuses
// with pod api object, and produce corresponding lifecycle events
/// TODO what about pause?
func (m *manager) compareAndProduceLifecycleEvents(apiPod *apiObject.Pod, runtimePodStatus *pod.PodStatus) {
	podUID := runtimePodStatus.ID
	m.podStatusRecords.UpdateRecord(podUID, runtimePodStatus)
	record := m.podStatusRecords.GetRecord(podUID)
	oldStatus, currentStatus := record.OldStatus, record.CurrentStatus

	// apiPod == nil means the pod is no longer existent, remove all the containers
	if apiPod == nil && oldStatus != nil {
		m.removeAllContainers(runtimePodStatus)
		m.podStatusRecords.RemoveRecord(podUID)
	}

	notIncludedMap := make(map[string]struct{})
	for _, c := range apiPod.Containers() {
		notIncludedMap[c.Name] = struct{}{}
	}

	for _, cs := range currentStatus.ContainerStatuses {
		parseSucc, containerName, _, _, _, _ := podutil.ParseContainerFullName(cs.Name)
		// illegal containerName, need remove it
		if !parseSucc {
			m.addNeedRemoveLifecycleEvent(podUID, cs.ID)
			continue
		}

		// Only deal with it when state has changed
		needDealWith := oldStatus == nil
		if !needDealWith {
			oldCs := oldStatus.GetContainerStatusByName(cs.Name)
			needDealWith = oldCs == nil || oldCs.State != cs.State
		}

		if needDealWith {
			switch cs.State {
			// It means this container is illegal, should be removed
			case container.ContainerStateRunning:
			case container.ContainerStateCreated:
				if apiPod.GetContainerByName(containerName) == nil {
					m.addNeedRemoveLifecycleEvent(podUID, cs.ID)
				}
			// Need restart it
			case container.ContainerStateExited:
				if apiPod.GetContainerByName(containerName) != nil {
					m.addNeedRestartLifecycleEvent(podUID, cs.ID)
				}
			default:
				m.addChangedLifecycleEvent(podUID, cs.ID)
			}
		}
		delete(notIncludedMap, containerName)
	}

	// Need to create all the container that has not been created
	for notInclude := range notIncludedMap {
		m.addNeedCreateAndStartLifecycleEvent(podUID, notInclude)
	}
}

func (m *manager) relist() error {
	// Step 1: Get all *runtime* pod statuses
	runtimePodStatuses, err := m.podManager.GetPodStatuses()
	if err != nil {
		return err
	}

	// Step 2: Get pod api object, and according to the api object, produce lifecycle events
	var apiPod *apiObject.Pod
	for podUID, runtimePodStatus := range runtimePodStatuses {
		apiPod = m.statusManager.GetPod(podUID)
		m.compareAndProduceLifecycleEvents(apiPod, runtimePodStatus)
	}

	return nil
}

func (m *manager) Updates() chan *PodLifecycleEvent {
	return m.eventCh
}

func (m *manager) Start() {
	ticker := time.Tick(relistIntervalSeconds * time.Second)
	for {
		select {
		case <-ticker:
			if err := m.relist(); err != nil {
				fmt.Println(err.Error())
			}
		}
	}
}
