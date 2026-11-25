package kube

// Helpers are cxz's own commands for a project pod and for the manager that
// runs it, where there is no docker exec --user and no guarantee the image has
// su, cp or chown. main runs them before its command line parser, since what
// follows them is someone else's command line. Registered on Linux only.
var Helpers = map[string]func(args []string) error{}

// RuntimeBinary is cxz inside a project pod.
const RuntimeBinary = "/cxz/tools/cxz"
