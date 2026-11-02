package main

import (
	"flag"
	"fmt"
	"github.com/iamvalson/blink/internal/auth"
	"os"
)

func main() {
	out := flag.String("out", ".secrets/jwt-signing-key.pem", "new private key file; existing files are never overwritten")
	flag.Parse()
	if err := auth.GenerateSigningKeyFile(*out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Created Ed25519 signing key file. Keep it private and persist it across API deployments.")
}
