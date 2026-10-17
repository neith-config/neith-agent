//go:build linux

package agent

func shmDir() string {
	return "/dev/shm"
}
