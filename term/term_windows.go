package term

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Start runs name in dir on a new pseudo console (Windows 10 1809 and later).
func Start(dir, name string, args ...string) (*Proc, error) {
	if strings.ContainsAny(name, `\/`) && !filepath.IsAbs(name) {
		name = filepath.Join(dir, name) // tools\setup.cmd is the repo's, as exec.Cmd's Dir makes it elsewhere
	}
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
	fail := func(err error) (*Proc, error) {
		in.Close()
		out.Close() // first: closing the console waits for its last output to be read otherwise
		windows.ClosePseudoConsole(hpc)
		return nil, err
	}

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fail(err)
	}
	defer attrs.Delete()
	// the attribute's value is the console handle itself, not a pointer to it
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		*(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return fail(err)
	}
	si := windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Flags = windows.STARTF_USESTDHANDLES // all null: not ours, the console's
	cmdline, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...)))
	wdir, _ := windows.UTF16PtrFromString(dir)
	var pi windows.ProcessInformation
	// suspended until it is in its job, so nothing it starts is outside it
	if err := windows.CreateProcess(nil, cmdline, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_SUSPENDED,
		nil, wdir, &si.StartupInfo, &pi); err != nil {
		return fail(err)
	}
	job := newJob(pi.Process)
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		windows.TerminateProcess(pi.Process, 1)
		windows.CloseHandle(pi.Thread)
		windows.CloseHandle(pi.Process)
		if job != 0 {
			windows.CloseHandle(job)
		}
		return fail(err)
	}
	windows.CloseHandle(pi.Thread)

	var mu sync.Mutex
	ended := false
	kill := func() {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case ended:
		case job != 0:
			windows.TerminateJobObject(job, 1)
		default: // no job: its tree, as far as taskkill can follow it
			Command("taskkill", "/T", "/F", "/PID", fmt.Sprint(pi.ProcessId)).Run()
		}
	}
	var code uint32
	done := make(chan struct{})
	go func() { // the output only ends once the console is closed
		windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		windows.GetExitCodeProcess(pi.Process, &code)
		mu.Lock()
		ended = true
		if job != 0 { // what it left running (a server it started, say) carries on, as from a terminal
			setJobLimits(job, false)
			windows.CloseHandle(job)
		}
		mu.Unlock()
		windows.CloseHandle(pi.Process)
		windows.ClosePseudoConsole(hpc)
		close(done)
	}()
	return &Proc{Reader: out, Writer: in, Pid: int(pi.ProcessId), kill: kill, Wait: func() int {
		<-done
		in.Close()
		out.Close()
		return int(code)
	}}, nil
}

// newJob puts process in a job of its own: everything it starts is in it too,
// so Kill stops them all at once (taskkill /T misses a process whose parent
// has already gone), and they stop with the installer if it is closed or
// dies mid-build. 0 if Windows won't; Kill falls back to taskkill.
func newJob(process windows.Handle) windows.Handle {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0
	}
	if setJobLimits(job, true) != nil || windows.AssignProcessToJobObject(job, process) != nil {
		windows.CloseHandle(job)
		return 0
	}
	return job
}

// setJobLimits: killOnClose ends the job's processes when its last handle
// (ours) closes. A process may always break away if it asks to (some
// installers do, and fail otherwise).
func setJobLimits(job windows.Handle, killOnClose bool) error {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if killOnClose {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	}
	_, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	return err
}

// Command is exec.Command without a console window flashing up.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd
}
