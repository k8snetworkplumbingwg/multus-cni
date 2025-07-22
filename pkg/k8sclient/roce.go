package k8sclient

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strconv"
    "strings"
    "syscall"

    v1 "k8s.io/api/core/v1"

    "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/logging"
    "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

// rdmaAllocationState represents a simple on-disk state file structure used to track
// allocated rdma<N> NAD indices per pod UID on the hosting node. This mimics the
// host-local style of tracking allocations in a local file and is intentionally
// simple (no compaction/cleanup besides release path).
type rdmaAllocationState struct {
    // Map of pod UID to list of allocated indices
    Pods map[string][]int `json:"pods"`
}

func rdmaStateFile(conf *types.NetConf) string {
    dir := conf.CNIDir
    if dir == "" {
        dir = "/var/lib/cni/multus" // fallback, though CNIDir should normally be set
    }
    return filepath.Join(dir, "roce_allocations.json")
}

// withRdmaState executes fn while holding an exclusive flock on the state file.
// If fn returns (true, nil) the state will be persisted back to disk.
func withRdmaState(conf *types.NetConf, fn func(st *rdmaAllocationState) (bool, error)) error {
    path := rdmaStateFile(conf)
    if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
        return err
    }
    f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
    if err != nil {
        return err
    }
    defer f.Close()
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        return fmt.Errorf("failed to lock rdma state file: %w", err)
    }
    defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

    st := &rdmaAllocationState{Pods: map[string][]int{}}
    if fi, _ := f.Stat(); fi != nil && fi.Size() > 0 {
        b, rerr := os.ReadFile(path)
        if rerr == nil {
            _ = json.Unmarshal(b, st)
            if st.Pods == nil {
                st.Pods = map[string][]int{}
            }
        }
    }
    changed, err := fn(st)
    if err != nil {
        return err
    }
    if !changed {
        return nil
    }
    // Truncate then write new json
    if _, err := f.Seek(0, 0); err != nil {
        return err
    }
    if err := f.Truncate(0); err != nil {
        return err
    }
    b, err := json.Marshal(st)
    if err != nil {
        return err
    }
    if _, err := f.Write(b); err != nil {
        return err
    }
    return f.Sync()
}

// allocateRdmaNADs allocates up to ifNum indices (0-7) and returns the list of indices.
// If the pod already has allocations, it will be made idempotent (re-using existing indices)
// and expanding/shrinking as necessary.
func allocateRdmaNADs(conf *types.NetConf, pod *v1.Pod, ifNum int) ([]int, error) {
    var result []int
    err := withRdmaState(conf, func(st *rdmaAllocationState) (bool, error) {
        uid := string(pod.UID)
        current := st.Pods[uid]
        if len(current) >= ifNum { // shrink if necessary
            st.Pods[uid] = current[:ifNum]
            result = st.Pods[uid]
            return true, nil
        }
        used := map[int]bool{}
        for _, lst := range st.Pods {
            for _, i := range lst {
                used[i] = true
            }
        }
        for i := 0; i < 8 && len(current) < ifNum; i++ {
            if !used[i] {
                current = append(current, i)
                used[i] = true
            }
        }
        if len(current) < ifNum {
            return false, fmt.Errorf("insufficient rdma NADs available: requested %d, allocated %d", ifNum, len(current))
        }
        st.Pods[uid] = current
        result = current
        return true, nil
    })
    return result, err
}

// ReleaseRdmaNADs releases any rdma indices allocated to the provided pod.
func ReleaseRdmaNADs(conf *types.NetConf, pod *v1.Pod) {
    if pod == nil {
        return
    }
    _ = withRdmaState(conf, func(st *rdmaAllocationState) (bool, error) {
        uid := string(pod.UID)
        if _, ok := st.Pods[uid]; !ok {
            return false, nil
        }
        delete(st.Pods, uid)
        return true, nil
    })
}

// ParseRdmaIfNum parses annotation value; exported for potential testing.
func ParseRdmaIfNum(annot string) (int, error) {
    if annot == "" {
        return 0, nil
    }
    v, err := strconv.Atoi(strings.TrimSpace(annot))
    if err != nil {
        return 0, err
    }
    if v < 0 {
        v = 0
    }
    if v > 8 {
        v = 8
    }
    return v, nil
}
