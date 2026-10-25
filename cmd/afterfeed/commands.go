package main

import (
	"errors"
	"fmt"
)

func authCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: afterfeed auth reset-2fa | reset-password | reset-vault-pin")
	}
	return fmt.Errorf("unknown auth command %q", args[0])
}

func indexCommand(args []string) error {
	if len(args) == 0 || args[0] != "rebuild" {
		return errors.New("usage: afterfeed index rebuild")
	}
	return errors.New("not implemented")
}
