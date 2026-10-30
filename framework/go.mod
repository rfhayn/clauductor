module github.com/rfhayn/stridal

go 1.24.0

// Go 1.26.0 to 1.26.4 instrument syscall.rawSyscall under -race on darwin, so a forked child
// crashes (tsan CHECK failed, exit 66) or spins before exec (golang/go#79804, fixed in 1.26.5;
// this repo's #84). With GOTOOLCHAIN=auto, the default, an older local Go switches to this one.
toolchain go1.26.9

require (
	github.com/coder/websocket v1.8.15
	github.com/creack/pty v1.1.24
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.0.10
)

require github.com/inconshreveable/mousetrap v1.1.0 // indirect
