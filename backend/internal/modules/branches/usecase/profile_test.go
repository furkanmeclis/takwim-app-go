package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/model"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
)

func TestSlugify(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"Kadıköy Şubesi":         "kadikoy-subesi",
		"  İSTANBUL / Çağlayan ": "istanbul-caglayan",
		"Ödül & Güneş -- 2":      "odul-gunes-2",
		"***":                    "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
	long := Slugify("a very long branch name that keeps going and going well past the sixty four char limit")
	if len(long) > maxSlugLen || long[len(long)-1] == '-' {
		t.Errorf("long slug %q", long)
	}
	if got := withSuffix(long, 12); len(got) > maxSlugLen {
		t.Errorf("suffix overflows: %q", got)
	}
}

func TestSlugGeneratedUniqueAndExplicit(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	svc := New(newFakeRepo(), nil)

	a, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("K1"), Name: strp("Kadıköy")})
	if err != nil || a.Slug != "kadikoy" {
		t.Fatalf("first slug = %q, %v", a.Slug, err)
	}
	b, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("K2"), Name: strp("Kadıköy")})
	if err != nil || b.Slug != "kadikoy-2" {
		t.Fatalf("second slug = %q, %v", b.Slug, err)
	}
	// Another organization may reuse the slug.
	if c, err := svc.Create(ctx, orgB, model.BranchInput{Code: strp("K1"), Name: strp("Kadıköy")}); err != nil || c.Slug != "kadikoy" {
		t.Fatalf("org B slug = %q, %v", c.Slug, err)
	}
	if _, err := svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Slug: strp("kadikoy")}); !errors.Is(err, ErrSlugTaken) {
		t.Fatalf("explicit taken slug: err = %v, want ErrSlugTaken", err)
	}
	var ve *ValidationError
	if _, err := svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Slug: strp("Kötü Slug")}); !errors.As(err, &ve) {
		t.Fatalf("invalid slug: err = %v, want validation", err)
	}
	got, err := svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Slug: strp("moda")})
	if err != nil || got.Slug != "moda" {
		t.Fatalf("rename slug = %q, %v", got.Slug, err)
	}
}

func TestTimezoneAndEffectiveZone(t *testing.T) {
	t.Parallel()
	ctx := orgctx.WithScope(context.Background(), orgctx.Scope{ID: orgA, Timezone: "Europe/Istanbul"})
	svc := New(newFakeRepo(), nil)

	b, err := svc.Create(ctx, orgA, model.BranchInput{Code: strp("B1"), Name: strp("Merkez")})
	if err != nil {
		t.Fatal(err)
	}
	if b.Timezone != nil || b.EffectiveTimezone != "Europe/Istanbul" || !b.ShowOnline {
		t.Fatalf("defaults: tz=%v effective=%q show_online=%v", b.Timezone, b.EffectiveTimezone, b.ShowOnline)
	}
	zone := "Europe/Berlin"
	b, err = svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Timezone: model.OptionalString{Set: true, Value: &zone}})
	if err != nil || b.Timezone == nil || *b.Timezone != zone || b.EffectiveTimezone != zone {
		t.Fatalf("own zone: %+v, %v", b, err)
	}
	bad := "Mars/Olympus"
	var ve *ValidationError
	if _, err := svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Timezone: model.OptionalString{Set: true, Value: &bad}}); !errors.As(err, &ve) {
		t.Fatalf("invalid zone: err = %v", err)
	}
	b, err = svc.Patch(ctx, orgA, b.UUID, model.BranchInput{Timezone: model.OptionalString{Set: true}})
	if err != nil || b.Timezone != nil || b.EffectiveTimezone != "Europe/Istanbul" {
		t.Fatalf("back to business zone: %+v, %v", b, err)
	}
}
