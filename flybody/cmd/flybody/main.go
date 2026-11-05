package main

import (
	"fmt"
	"os"

	"github.com/jhinw/flybrain/flybody/internal/app/runtime"
	"github.com/jhinw/flybrain/flybody/internal/domain/body"
	"github.com/jhinw/flybrain/flybody/internal/domain/brain"
	"github.com/jhinw/flybrain/flybody/internal/infra/cli"
	"github.com/jhinw/flybrain/flybody/internal/infra/store"
)

func main() {
	virtualBody := body.NewVirtualBody("peer-8f3a")
	connectome := brain.NewConceptConnectome()
	snapStore := store.NewJSONSnapshotStore()
	rt := runtime.NewPhysiologyRuntime(virtualBody, connectome, snapStore)

	app := cli.NewCLIApp(rt, cli.DefaultStateFile, os.Stdout)
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
