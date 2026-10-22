package skillsapi_test

import (
	"testing"

	"github.com/metorial/cli/internal/skillsapi/skillstest"
)

func TestResolveSkillResolvesSlugDirectly(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.AddSkills(250)

	skill, err := fake.Client(t).ResolveSkill(skillstest.SkillSlug)
	if err != nil {
		t.Fatalf("ResolveSkill() error = %v", err)
	}
	if skill.ID != skillstest.SkillID {
		t.Fatalf("resolved %s, want %s", skill.ID, skillstest.SkillID)
	}
	if fake.SkillLists != 0 {
		t.Fatalf("listed skills %d times, want 0", fake.SkillLists)
	}
}

func TestResolveSkillFallsBackToPagingOnLegacyBackends(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.AddSkills(250)
	fake.LegacySkillLookup = true

	skill, err := fake.Client(t).ResolveSkill(skillstest.SkillSlug)
	if err != nil {
		t.Fatalf("ResolveSkill() error = %v", err)
	}
	if skill.ID != skillstest.SkillID {
		t.Fatalf("resolved %s, want %s", skill.ID, skillstest.SkillID)
	}

	if _, err := fake.Client(t).ResolveSkill("missing-skill"); err == nil {
		t.Fatal("ResolveSkill() should fail for an unknown slug")
	}
}

func TestResolveSkillStopsWhenTheCursorDoesNotAdvance(t *testing.T) {
	fake := skillstest.NewServer(t)
	fake.AddSkills(250)
	fake.LegacySkillLookup = true
	fake.IgnoreAfterCursor = true

	if _, err := fake.Client(t).ResolveSkill("missing-skill"); err == nil {
		t.Fatal("ResolveSkill() should fail for an unknown slug")
	}
	if fake.SkillLists != 2 {
		t.Fatalf("listed skills %d times, want 2", fake.SkillLists)
	}
}
