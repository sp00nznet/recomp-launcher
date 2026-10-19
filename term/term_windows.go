package term

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Start runs name in dir on a new pseudo console (Windows 10 1809 and later).
func Start(dir, name string, args ...string) (*Proc, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, err
	}
	var ptyIn, ourIn, ourOut, ptyOut windows.Handle
	if err := windows.CreatePipe(&ptyIn, &ourIn, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&ourOut, &ptyOut, nil, 0); err != nil {
		windows.CloseHandle(ptyIn)
		windows.CloseHandle(ourIn)
		return nil, err
	}
	var hpc windows.Handle
	// wide, so the console doesn't wrap long lines and redraw them
	err = windows.CreatePseudoConsole(windows.Coord{X: 250, Y: 50}, ptyIn, ptyOut, 0, &hpc)
	windows.CloseHandle(ptyIn)
	windows.CloseHandle(ptyOut)
	in, out := os.NewFile(uintptr(ourIn), "conin"), os.NewFile(uintptr(ourOut), "conout")
	if err != nil {
		in.Close()
		out.Close()
		return nil, fmt.Errorf("no pseudo console (Windows 10 1809 or later is needed): %w", err)
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	// the attribute's value is the console handle itself, not a pointer to it
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return nil, err
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES // all null: not ours, the console's
	cmdline, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...)))
	wdir, _ := windows.UTF16PtrFromString(dir)
	var pi windows.ProcessInformation
	if err := windows.CreateProcess(nil, cmdline, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		nil, wdir, &si.StartupInfo, &pi); err != nil {
		windows.ClosePseudoConsole(hpc)
		in.Close()
		out.Close()
		return nil, err
	}
	windows.CloseHandle(pi.Thread)

	var code uint32
	done := make(chan struct{})
	go func() { // the output only ends once the console is closed
		windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		windows.GetExitCodeProcess(pi.Process, &code)
		windows.CloseHandle(pi.Process)
		windows.ClosePseudoConsole(hpc)
		close(done)
	}()
	return &Proc{Reader: out, Writer: in, Pid: int(pi.ProcessId), Wait: func() int {
		<-done
		in.Close()
		out.Close()
		return int(code)
	}}, nil
}

// Kill stops the command and everything it started: cmd, PowerShell, the compiler.
func Kill(pid int) {
	Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pid)).Run()
}

// Command is exec.Command without a console window flashing up.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}
