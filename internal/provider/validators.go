package provider

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

// noPathSegment rejects empty names, path separators, and ".." traversal.
func noPathSegment() validator.String {
	return stringvalidator.All(
		stringvalidator.LengthAtLeast(1),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/\0]+$`),
			"must be a single path segment (no '/' or null bytes)",
		),
		pathSegmentNoDotDot(),
	)
}

type pathSegmentNoDotDotValidator struct{}

func pathSegmentNoDotDot() validator.String { return pathSegmentNoDotDotValidator{} }

func (pathSegmentNoDotDotValidator) Description(_ context.Context) string {
	return "must not be '.' or '..'"
}
func (pathSegmentNoDotDotValidator) MarkdownDescription(ctx context.Context) string {
	return pathSegmentNoDotDotValidator{}.Description(ctx)
}
func (pathSegmentNoDotDotValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	v := req.ConfigValue.ValueString()
	if v == "." || v == ".." || path.Clean(v) != v {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid path segment",
			fmt.Sprintf("%q is not a safe single path segment", v),
		)
	}
}

func unitNameValidator() validator.String {
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/@]+(@[^/@]*)?\.(service|socket|timer|path|mount|automount|swap|target|slice|scope|device)$`),
			"must end with a known systemd unit suffix (e.g. .service, .timer, .socket)",
		),
	)
}

func templateNameValidator() validator.String {
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/@]+@\.(service|socket|timer|path|mount|automount|swap|target|slice|scope|device)$`),
			"must be a template unit name (e.g. app@.service)",
		),
	)
}

func instanceStringValidator() validator.String {
	return stringvalidator.All(
		stringvalidator.LengthAtLeast(1),
		stringvalidator.RegexMatches(
			mustCompile(`^[^/@]+$`),
			"instance must not contain '@' or '/'",
		),
	)
}

func unitSuffixValidator(suffix string) validator.String {
	esc := strings.ReplaceAll(suffix, ".", `\.`)
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(esc+`$`),
			fmt.Sprintf("unit name must end with %s", suffix),
		),
	)
}

func dropinNameValidator() validator.String {
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(`\.conf$`),
			"drop-in filename must end with .conf",
		),
	)
}

func networkFilenameValidator(suffix string) validator.String {
	esc := strings.ReplaceAll(suffix, ".", `\.`)
	return stringvalidator.All(
		noPathSegment(),
		stringvalidator.RegexMatches(
			mustCompile(esc+`$`),
			fmt.Sprintf("filename must end with %s", suffix),
		),
	)
}

func portValidator() validator.Int64 {
	return int64validator.Between(1, 65535)
}

func nonEmptyContent() validator.String {
	return stringvalidator.LengthAtLeast(1)
}

func credentialKeySourceValidator() validator.String {
	return stringvalidator.OneOf("auto", "host", "tpm2", "host+tpm2")
}
