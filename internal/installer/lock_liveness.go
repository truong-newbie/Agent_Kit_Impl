package installer

import "github.com/truong-newbie/Agent_Kit_Impl/internal/paths"

func processLiveness(pid int) string {
	return paths.ProcessLiveness(pid)
}
