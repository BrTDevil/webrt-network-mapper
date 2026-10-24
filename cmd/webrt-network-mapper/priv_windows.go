//go:build windows

package main

func fixOwnership(dir string) {}

func rawARPStatus() string { return "indisponibil pe Windows — se folosește ping + tabela ARP" }
