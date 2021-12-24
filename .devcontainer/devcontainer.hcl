name = "Conductor"
image = "dev-golang:latest"
runArgs = [
    "--cap-add=SYS_PTRACE",
    "--security-opt",
    "seccomp=unconfined",
]
settings = {
    "go.toolsManagement.checkForUpdates" = "local"
    "go.useLanguageServer" = true
    "go.gopath" = "/go"
    "go.goroot" = "/usr/local/go"
}
extensions = [
    "golang.Go",
]
postCreateCommand = "go version"
remoteUser = "vscode"
