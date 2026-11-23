// Package limits holds the ceilings compiled into both run_ephemeral_container and
// ephemeral-authz (kept equal, USS-005 open item: values provisional). SSR-AGE-008.
package limits

const (
	MaxMemoryBytes int64 = 2 << 30       // 2 GiB
	MaxNanoCPUs    int64 = 2_000_000_000 // 2 CPUs
	MaxPids        int64 = 512
	MaxTimeoutS    int   = 3600
	MaxOutBytes    int64 = 1 << 30   // out_max ceiling
	WorkTmpfsBytes int64 = 512 << 20 // /work
	TmpTmpfsBytes  int64 = 512 << 20 // /tmp
	HomeTmpfsBytes int64 = 128 << 20 // /home/run
	MaxStageBytes  int64 = 1 << 30   // staging size cap (exc-5)
	LockWaitMillis int   = 2000      // flock bound before `busy`
	// Agent container bounds for the slot accounting (SSR-AGC-012), provisional.
	AgentMemoryBytes int64 = 4 << 30
	AgentNanoCPUs    int64 = 4_000_000_000
	AgentPids        int64 = 2048
	AgentUID               = 1001
	AgentGID               = 1001
)
