package status

import (
	"minik8s/apiObject"
	"minik8s/kubelet/src/types"
	"sync"
)

type podCache struct {
	cacheLock sync.RWMutex
	cache     map[types.UID]*apiObject.Pod
}

func (pc *podCache) updateInternal(podUID types.UID, newPod *apiObject.Pod) {
	pc.cacheLock.Lock()
	defer pc.cacheLock.Unlock()
	pc.cache[podUID] = newPod
}

func (pc *podCache) getInternal(podUID types.UID) *apiObject.Pod {
	pc.cacheLock.RLock()
	defer pc.cacheLock.RUnlock()
	if pod, exists := pc.cache[podUID]; exists {
		return pod
	}
	return nil
}

func (pc *podCache) GetPod(podUID types.UID) *apiObject.Pod {
	return pc.getInternal(podUID)
}

func (pc *podCache) UpdatePod(podUID types.UID, newPod *apiObject.Pod) {
	pc.updateInternal(podUID, newPod)
}

func newPodCache() *podCache {
	return &podCache{
		cacheLock: sync.RWMutex{},
		cache:     make(map[types.UID]*apiObject.Pod),
	}
}

type Manager interface {
	GetPod(podUID types.UID) *apiObject.Pod
	UpdatePod(podUID types.UID, newPod *apiObject.Pod)
}

type manager struct {
	cache *podCache
}

func (m *manager) GetPod(podUID types.UID) *apiObject.Pod {
	return m.cache.GetPod(podUID)
}

func (m *manager) UpdatePod(podUID types.UID, newPod *apiObject.Pod) {
	m.cache.UpdatePod(podUID, newPod)
}

func NewStatusManager() Manager {
	return &manager{
		cache: newPodCache(),
	}
}
