package gonut

import (
	"testing"
	"unsafe"
)

func TestModule_ToBytes(t *testing.T) {

	var module Module

	t.Logf("%x", unsafe.Sizeof(module.DonutModule))
	t.Logf("%x", unsafe.Offsetof(module.DonutModule.Args))
}
