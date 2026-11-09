// Module ptycheck runs the example programs in a real pseudo-terminal and
// checks the screen they draw. It is a separate module so the library itself
// keeps no test-only dependencies.
module github.com/tolaniverse/opentui-go/tools/ptycheck

go 1.26.2

require github.com/creack/pty v1.1.24

require github.com/rivo/uniseg v0.4.7

require golang.org/x/sys v0.49.0
