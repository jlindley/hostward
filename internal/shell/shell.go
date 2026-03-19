package shell

import (
	"fmt"
	"strings"

	"hostward/internal/state"
)

type BannerMode string

const (
	BannerCount BannerMode = "count"
	BannerList  BannerMode = "list"
)

func Banner(snapshot state.Snapshot, mode BannerMode) string {
	if snapshot.TotalCount == 0 && snapshot.FailingCount == 0 {
		return "hostward: 0 monitors"
	}

	if snapshot.FailingCount > 0 {
		switch mode {
		case BannerList:
			if len(snapshot.Failing) > 0 {
				return "hostward: failing: " + summarizeFailing(snapshot.Failing)
			}
		}

		return fmt.Sprintf("hostward: failing: %d of %d", snapshot.FailingCount, snapshot.TotalCount)
	}

	if snapshot.UnknownCount > 0 {
		if snapshot.StatusCounts.OK > 0 {
			return fmt.Sprintf("hostward: %d ok, %d unknown", snapshot.StatusCounts.OK, snapshot.UnknownCount)
		}

		return fmt.Sprintf("hostward: %d unknown", snapshot.UnknownCount)
	}

	return fmt.Sprintf("hostward: %d ok", snapshot.TotalCount)
}

func FailingCount(snapshot state.Snapshot) string {
	return fmt.Sprintf("%d", snapshot.FailingCount)
}

func Snippet(target string) (string, error) {
	switch target {
	case "zsh":
		return zshSnippet, nil
	case "bash":
		return bashSnippet, nil
	default:
		return "", fmt.Errorf("unsupported shell %q", target)
	}
}

func summarizeFailing(names []string) string {
	const maxNames = 3
	if len(names) <= maxNames {
		return strings.Join(names, ", ")
	}

	return fmt.Sprintf("%s, +%d more", strings.Join(names[:maxNames], ", "), len(names)-maxNames)
}

const zshSnippet = `__hostward_precmd() {
  local count
  count="$(hostward env failing-count 2>/dev/null)" || count=0
  export HOSTWARD_FAILING_COUNT="${count:-0}"
}

if [[ -o interactive ]]; then
  hostward banner 2>/dev/null
  __hostward_precmd
  if (( ${precmd_functions[(Ie)__hostward_precmd]} == 0 )); then
    precmd_functions+=(__hostward_precmd)
  fi
fi
`

const bashSnippet = `__hostward_prompt_command() {
  local count
  count="$(hostward env failing-count 2>/dev/null)" || count=0
  export HOSTWARD_FAILING_COUNT="${count:-0}"
}

if [[ $- == *i* ]]; then
  hostward banner 2>/dev/null
  __hostward_prompt_command
  case ";${PROMPT_COMMAND};" in
    *";__hostward_prompt_command;"*) ;;
    *) PROMPT_COMMAND="__hostward_prompt_command${PROMPT_COMMAND:+;${PROMPT_COMMAND}}" ;;
  esac
fi
`
