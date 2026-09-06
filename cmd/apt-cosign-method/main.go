// Command apt-cosign-method is installed as /usr/lib/apt/methods/sigstore+https,
// an apt acquire-method that verifies Sigstore bundles for fetched targets
// before handing them back to apt.
package main

import (
	"context"
	"fmt"
	"os"

	"apt-cosign/internal/method"
)

func main() {
	logger := method.OpenDebugLog()
	m := method.New(os.Stdin, os.Stdout, logger)
	if err := m.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "apt-cosign-method:", err)
		os.Exit(1)
	}
}
