// Command terraform-provider-fixture is a stand-in provider binary that the
// release workflow's dry-run builds, packages and signs.
package main

import "fmt"

var (
	version = "dev"
	commit  = "none"
)

func main() {
	fmt.Println(version, commit)
}
