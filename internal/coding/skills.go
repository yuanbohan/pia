package coding

import (
	"fmt"

	"github.com/yuanbohan/pia/internal/coding/skills"
)

// SkillDiagnostic is one bounded project Skill discovery warning.
type SkillDiagnostic = skills.Diagnostic

func discoverPiaSkills(workspace *Workspace) (skills.Discovery, error) {
	if workspace == nil || workspace.Root() == nil {
		return skills.Discovery{}, fmt.Errorf("coding: discover Pia skills: workspace is required")
	}
	return skills.Discover(workspace.Root())
}
