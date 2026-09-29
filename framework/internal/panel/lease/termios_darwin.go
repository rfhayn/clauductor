//go:build darwin

package lease

import "syscall"

// The termios ioctls lock-run uses to save and restore a terminal it hands over.
const ioctlGetTermios, ioctlSetTermios = syscall.TIOCGETA, syscall.TIOCSETA
