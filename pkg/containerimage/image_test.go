package containerimage

import "testing"

const validImage = "docker.io/example/tool:1.2.3@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestValidateDigestPinnedImage(t *testing.T) {
	if err := Validate(validImage); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsMutableAndMalformedReferences(t *testing.T) {
	for _, image := range []string{
		"example/tool:latest",
		"not a valid image@@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"example/tool@sha512:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"example/tool@sha256:short",
	} {
		if err := Validate(image); err == nil {
			t.Fatalf("Validate(%q) succeeded", image)
		}
	}
}

func TestValidateTagOnlyPreservesExplicitTrimmedReference(t *testing.T) {
	for _, image := range []string{
		"busybox:1.37",
		"docker.io/library/busybox:1.37",
		"example.invalid/org/nested/tool:v1",
		"registry.example:5000/org/image:tag",
	} {
		got, err := ValidateTagOnly(" \t" + image + "\n")
		if err != nil || got != image {
			t.Fatalf("ValidateTagOnly(%q) = %q, %v", image, got, err)
		}
	}
}

func TestValidateTagOnlyRejectsUntaggedDigestAndMalformedReferences(t *testing.T) {
	for _, image := range []string{"", "busybox", "example.invalid/tool", validImage, "not a valid image"} {
		if _, err := ValidateTagOnly(image); err == nil {
			t.Fatalf("ValidateTagOnly(%q) succeeded", image)
		}
	}
}

func TestValidatePinnedTagOnlyRequiresExactNonLatestTag(t *testing.T) {
	const image = "ghcr.io/openclaw/openclaw:2026.7.1"
	if err := ValidatePinnedTagOnly(image); err != nil {
		t.Fatalf("ValidatePinnedTagOnly(%q): %v", image, err)
	}
	for _, invalid := range []string{
		"",
		" " + image,
		image + "\n",
		"ghcr.io/openclaw/openclaw",
		"ghcr.io/openclaw/openclaw:latest",
		validImage,
		"not a valid image",
	} {
		if err := ValidatePinnedTagOnly(invalid); err == nil {
			t.Fatalf("ValidatePinnedTagOnly(%q) succeeded", invalid)
		}
	}
}

func TestValidateTagOnlyStillTrimsAndAcceptsLatest(t *testing.T) {
	const image = "example.invalid/task:latest"
	if got, err := ValidateTagOnly(" \t" + image + "\n"); err != nil || got != image {
		t.Fatalf("ValidateTagOnly() = %q, %v", got, err)
	}
}

func TestValidatePinnedTagAcceptsOptionalDigestWithoutWeakeningTagRules(t *testing.T) {
	const tag = "docker.io/example/tool:1.2.3"
	const digest = "@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	for _, image := range []string{tag, tag + digest} {
		if err := ValidatePinnedTag(image); err != nil {
			t.Fatalf("ValidatePinnedTag(%q): %v", image, err)
		}
	}
	for _, image := range []string{
		"", " " + tag, tag + "\n", " " + tag + digest, tag + digest + "\n",
		"docker.io/example/tool", "docker.io/example/tool" + digest,
		"docker.io/example/tool:latest", "docker.io/example/tool:latest" + digest,
		tag + "@", tag + "@sha256:short",
		"not a valid image",
	} {
		if err := ValidatePinnedTag(image); err == nil {
			t.Fatalf("ValidatePinnedTag(%q) succeeded", image)
		}
	}
}
