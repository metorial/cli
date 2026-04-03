module github.com/metorial/cli

go 1.25.0

require (
	github.com/manifoldco/promptui v0.9.0
	github.com/metorial/metorial-go v0.0.0
	github.com/pelletier/go-toml/v2 v2.2.4
	github.com/spf13/cobra v1.10.1
	golang.org/x/term v0.32.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/modelcontextprotocol/go-sdk v1.4.1 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.34.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
)

replace github.com/metorial/metorial-go => ../clients/metorial-go
