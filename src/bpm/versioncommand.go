package main;

import (
    "fmt"
)

type VersionCommand struct {
}

func (cmd *VersionCommand) Execute() (error) {
    fmt.Println("bpm version 1.0.5")
    return nil
}
