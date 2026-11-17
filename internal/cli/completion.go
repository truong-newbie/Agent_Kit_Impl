package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/truong-newbie/Agent_Kit_Impl/internal/apperr"
)

const bashCompletionScript = `# bash completion for dtkit
_dtkit_complete() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    local cmds="version help list info validate create fork pack doctor diff install status update remove recover project check hooks hook completion"
    local runtimes="codex gemini opencode cursor claude"
    local presets="minimal review-only engineering"

    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "$cmds" -- "$cur") )
        return 0
    fi

    case "$prev" in
        --runtime)
            COMPREPLY=( $(compgen -W "$runtimes" -- "$cur") )
            return 0
            ;;
        --preset)
            COMPREPLY=( $(compgen -W "$presets" -- "$cur") )
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh powershell" -- "$cur") )
            return 0
            ;;
        project)
            COMPREPLY=( $(compgen -W "inspect init" -- "$cur") )
            return 0
            ;;
        hooks)
            COMPREPLY=( $(compgen -W "list status enable disable" -- "$cur") )
            return 0
            ;;
        create)
            COMPREPLY=( $(compgen -W "kit skill" -- "$cur") )
            return 0
            ;;
    esac

    local flags="--project --global --runtime --preset --components --path --from --source --sha256 --offline --dry-run --diff --yes --json --redact --changed --all --allow-autofix --as --dir --version --same-id"
    COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
}
complete -F _dtkit_complete dtkit
`

const zshCompletionScript = `#compdef dtkit
_dtkit() {
    local -a cmds runtimes presets
    cmds=(
        'version:Print version metadata'
        'help:Show CLI help'
        'list:List bundled kits'
        'info:Inspect kit package metadata'
        'validate:Validate a kit directory or archive'
        'create:Create kit or skill scaffolding'
        'fork:Fork an existing kit to a local customizable directory'
        'pack:Build reproducible kit tarball'
        'doctor:Run diagnostic health checks'
        'diff:Preview bounded content diff and ownership'
        'install:Install a kit into project or global scope'
        'status:Show installed kit status'
        'update:Update installed kit'
        'remove:Remove an installed kit safely'
        'recover:Recover unfinished transactions'
        'project:Inspect or initialize .dtkit/project.json'
        'check:Run trusted project quality checks'
        'hooks:Manage opt-in runtime hooks'
        'hook:Execute a registered hook handler'
        'completion:Generate shell completion scripts'
    )
    runtimes=(codex gemini opencode cursor claude)
    presets=(minimal review-only engineering)

    if (( CURRENT == 2 )); then
        _describe 'command' cmds
        return
    fi

    case "$words[CURRENT-1]" in
        --runtime) _values 'runtime' $runtimes ;;
        --preset) _values 'preset' $presets ;;
        completion) _values 'shell' bash zsh powershell ;;
        project) _values 'subcommand' inspect init ;;
        hooks) _values 'subcommand' list status enable disable ;;
        create) _values 'kind' kit skill ;;
    esac
}
compdef _dtkit dtkit
`

const powerShellCompletionScript = `# PowerShell completion for dtkit
Register-ArgumentCompleter -Native -CommandName dtkit -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $tokens = $commandAst.CommandElements | ForEach-Object { $_.Extent.Text }
    $cmds = @('version','help','list','info','validate','create','fork','pack','doctor','diff','install','status','update','remove','recover','project','check','hooks','hook','completion')
    $runtimes = @('codex','gemini','opencode','cursor','claude')
    $presets = @('minimal','review-only','engineering')
    $flags = @('--project','--global','--runtime','--preset','--components','--path','--from','--source','--sha256','--offline','--dry-run','--diff','--yes','--json','--redact','--changed','--all','--allow-autofix','--as','--dir','--version','--same-id')

    if ($tokens.Count -le 2 -and ($wordToComplete -eq $tokens[-1] -or $tokens.Count -eq 1)) {
        return $cmds | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
    }
    $prev = if ($wordToComplete -and $tokens.Count -ge 2) { $tokens[-2] } else { $tokens[-1] }
    $candidates = switch ($prev) {
        '--runtime' { $runtimes }
        '--preset' { $presets }
        'completion' { @('bash','zsh','powershell') }
        'project' { @('inspect','init') }
        'hooks' { @('list','status','enable','disable') }
        'create' { @('kit','skill') }
        default { $flags }
    }
    $candidates | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`

func runCompletion(args []string, deps Deps) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return apperr.Usage("missing_shell", "usage: dtkit completion <bash|zsh|powershell>")
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "bash":
		_, err := io.WriteString(deps.Stdout, bashCompletionScript)
		return err
	case "zsh":
		_, err := io.WriteString(deps.Stdout, zshCompletionScript)
		return err
	case "powershell", "pwsh":
		_, err := io.WriteString(deps.Stdout, powerShellCompletionScript)
		return err
	default:
		return apperr.Usage("unsupported_shell", fmt.Sprintf("unsupported shell %q (supported: bash, zsh, powershell)", args[0]))
	}
}
