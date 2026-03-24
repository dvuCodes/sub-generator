//go:build !windows

package main

import "os"

func initJobObject() {}

func assignToJob(_ *os.Process) {}
