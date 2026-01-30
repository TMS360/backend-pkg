package main

import (
	"fmt"
	"os"

	"github.com/TMS360/backend-pkg/graphql"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: copyschema <destination-dir>")
		os.Exit(1)
	}

	if err := graphql.CopySchemas(os.Args[1]); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("GraphQL schemas copied successfully!")
}
