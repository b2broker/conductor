package conductor

type Version string

type CoreVersion struct {
	Major uint32
	Minor uint32
	Patch uint32
}

type SemVer struct {
	Core       CoreVersion
	PreRelease string
	Build      string
}

func (v *Version) GetSemVer() *SemVer {
	return &SemVer{}
}

type ResourceName string

type DockerImage string

type KeyEqualValue *string

type Resource struct {
	Name        ResourceName
	Image       DockerImage
	Environment []KeyEqualValue
}

type ConductorConfig struct {
	Version   Version
	Resources []Resource
}
