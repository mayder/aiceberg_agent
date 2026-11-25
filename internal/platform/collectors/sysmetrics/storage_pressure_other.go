//go:build !linux

package sysmetrics

type storageSampler struct{}

func (s *storageSampler) sample() *storageProfile { return nil }
