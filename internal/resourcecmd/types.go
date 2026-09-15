package resourcecmd

type OperationName string

const (
	OperationList          OperationName = "list"
	OperationGet           OperationName = "get"
	OperationCreate        OperationName = "create"
	OperationUpdate        OperationName = "update"
	OperationDelete        OperationName = "delete"
	OperationApprove       OperationName = "approve"
	OperationDeny          OperationName = "deny"
	OperationRevoke        OperationName = "revoke"
	OperationAddListing    OperationName = "add-listing"
	OperationRemoveListing OperationName = "remove-listing"
	OperationGetLogs       OperationName = "get-logs"
	OperationGetSchema     OperationName = "get-schema"
)

type FlagType string

const (
	FlagString      FlagType = "string"
	FlagBool        FlagType = "bool"
	FlagInt         FlagType = "int"
	FlagFloat       FlagType = "float"
	FlagStringSlice FlagType = "string-slice"
	FlagJSON        FlagType = "json"
	FlagJSONFile    FlagType = "json-file"
)

type ArgumentSpec struct {
	Name        string
	Target      string
	Required    bool
	Description string
}

type FlagSpec struct {
	Name      string
	Type      FlagType
	Target    string
	Usage     string
	Required  bool
	Repeated  bool
	Shorthand string
}

type OperationSpec struct {
	Name       OperationName
	Method     string
	Path       string
	Use        string
	Short      string
	Long       string
	Args       []ArgumentSpec
	Flags      []FlagSpec
	Examples   []string
	SeeAlso    []string
	SDKMapping string
}

type ShortcutSpec struct {
	Path   []string
	Method OperationName
}

type ResourceSpec struct {
	Plural     string
	Singular   string
	Short      string
	Long       string
	PathPlural string
	Operations []OperationSpec
	Aliases    []string
	Children   []ResourceSpec
	Shortcuts  []ShortcutSpec
}
