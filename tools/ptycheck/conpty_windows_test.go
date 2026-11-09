//go:build windows

package ptycheck

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// conPTY runs a program in a Windows pseudo console (ConPTY), the Windows
// counterpart of a Unix PTY. Its output is re-encoded by the console host,
// so the screen emulator sees ConPTY's escape sequences, not the program's.
type conPTY struct {
	console windows.Handle
	process windows.Handle
	thread  windows.Handle
	in      *os.File // typed input to the program
	out     *os.File // what the program draws
}

func startTerminal(bin string, env []string, w, h int) (terminal, error) {
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("input pipe: %w", err)
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		return nil, fmt.Errorf("output pipe: %w", err)
	}
	var console windows.Handle
	size := windows.Coord{X: int16(w), Y: int16(h)}
	if err := windows.CreatePseudoConsole(size, inRead, outWrite, 0, &console); err != nil {
		windows.CloseHandle(inRead)
		windows.CloseHandle(inWrite)
		windows.CloseHandle(outRead)
		windows.CloseHandle(outWrite)
		return nil, fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	// Keep inRead and outWrite open until CreateProcess returns. ConPTY
	// duplicates them, but closing them first can drop the session.
	closeRest := func() {
		if console != 0 {
			windows.ClosePseudoConsole(console)
			console = 0
		}
		if inWrite != 0 {
			windows.CloseHandle(inWrite)
			inWrite = 0
		}
		if outRead != 0 {
			windows.CloseHandle(outRead)
			outRead = 0
		}
		if inRead != 0 {
			windows.CloseHandle(inRead)
			inRead = 0
		}
		if outWrite != 0 {
			windows.CloseHandle(outWrite)
			outWrite = 0
		}
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		closeRest()
		return nil, err
	}
	defer attrs.Delete()
	// UpdateProcThreadAttribute takes the HPCON value itself, not &HPCON.
	// https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&console)), unsafe.Sizeof(console)); err != nil {
		closeRest()
		return nil, fmt.Errorf("pseudo console attribute: %w", err)
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	// Null std handles, explicitly. Otherwise a parent with redirected
	// pipes (CI) is inherited and the child is not a console, even though
	// the pseudoconsole attribute is set. node-pty does the same.
	// https://learn.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session
	si.Flags = windows.STARTF_USESTDHANDLES
	si.StdInput, si.StdOutput, si.StdErr = 0, 0, 0

	cmdline, err := windows.UTF16PtrFromString(syscall.EscapeArg(bin))
	if err != nil {
		closeRest()
		return nil, err
	}
	block := envBlock(env)
	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	err = windows.CreateProcess(nil, cmdline, nil, nil, false, flags, block, nil, &si.StartupInfo, &pi)
	runtime.KeepAlive(block)
	if err != nil {
		closeRest()
		return nil, fmt.Errorf("CreateProcess: %w", err)
	}
	// The pseudo console holds its own copies of these ends.
	windows.CloseHandle(inRead)
	windows.CloseHandle(outWrite)
	inRead, outWrite = 0, 0
	return &conPTY{
		console: console, process: pi.Process, thread: pi.Thread,
		in: os.NewFile(uintptr(inWrite), "conpty-in"), out: os.NewFile(uintptr(outRead), "conpty-out"),
	}, nil
}

// envBlock encodes env as a CREATE_UNICODE_ENVIRONMENT block.
func envBlock(env []string) *uint16 {
	if len(env) == 0 {
		return nil
	}
	block := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	return &block[0]
}

func (c *conPTY) Read(p []byte) (int, error)  { return c.out.Read(p) }
func (c *conPTY) Write(p []byte) (int, error) { return c.in.Write(p) }

func (c *conPTY) Resize(w, h int) error {
	return windows.ResizePseudoConsole(c.console, windows.Coord{X: int16(w), Y: int16(h)})
}

func (c *conPTY) Kill() { windows.TerminateProcess(c.process, 1) }

func (c *conPTY) Wait() error {
	if _, err := windows.WaitForSingleObject(c.process, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(c.process, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("exit status %d", code)
	}
	return nil
}

// Close ends the pseudo console (which flushes its last output to the
// reader), then the pipes and process handles.
func (c *conPTY) Close() error {
	windows.ClosePseudoConsole(c.console)
	c.in.Close()
	c.out.Close()
	windows.CloseHandle(c.thread)
	return windows.CloseHandle(c.process)
}
