package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	modkernel32              = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW     = modkernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObj = modkernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob   = modkernel32.NewProc("AssignProcessToJobObject")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x00002000
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformationT struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var jobHandle syscall.Handle

func initJobObject() {
	handle, _, err := procCreateJobObjectW.Call(0, 0)
	if handle == 0 {
		fmt.Fprintf(os.Stderr, "warning: failed to create job object: %v\n", err)
		return
	}

	info := jobObjectExtendedLimitInformationT{}
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose

	ret, _, err := procSetInformationJobObj.Call(
		handle,
		uintptr(jobObjectExtendedLimitInformation),
		uintptr(unsafe.Pointer(&info)),
		uintptr(unsafe.Sizeof(info)),
	)
	if ret == 0 {
		fmt.Fprintf(os.Stderr, "warning: failed to set job object info: %v\n", err)
		syscall.CloseHandle(syscall.Handle(handle))
		return
	}

	jobHandle = syscall.Handle(handle)
}

func assignToJob(process *os.Process) {
	if jobHandle == 0 || process == nil {
		return
	}

	const processSetQuota = 0x0100
	handle, err := syscall.OpenProcess(
		processSetQuota|syscall.PROCESS_TERMINATE,
		false,
		uint32(process.Pid),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to open process %d for job assignment: %v\n", process.Pid, err)
		return
	}
	defer syscall.CloseHandle(handle)

	ret, _, err := procAssignProcessToJob.Call(uintptr(jobHandle), uintptr(handle))
	if ret == 0 {
		fmt.Fprintf(os.Stderr, "warning: failed to assign process %d to job: %v\n", process.Pid, err)
	}
}
