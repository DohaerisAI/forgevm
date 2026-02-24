package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"

	serverURL string
	apiKey    string
)

func main() {
	root := &cobra.Command{
		Use:   "forgevm",
		Short: "ForgeVM — microVM sandbox orchestrator for LLMs",
		Long:  "ForgeVM gives any LLM the ability to spawn isolated sandboxes, execute code, manage files, and tear it all down.",
	}

	root.PersistentFlags().StringVar(&serverURL, "server", "http://localhost:7423", "ForgeVM server URL")
	root.PersistentFlags().StringVar(&apiKey, "api-key", os.Getenv("FORGEVM_API_KEY"), "API key for authentication")

	root.AddCommand(
		newServeCmd(),
		newSpawnCmd(),
		newExecCmd(),
		newKillCmd(),
		newListCmd(),
		newVersionCmd(),
		newTUICmd(),
		newBuildImageCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func getClient() *httpClient {
	return newHTTPClient(serverURL, apiKey)
}
