package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func stringOneOf(values []string) validator.String {
	return stringvalidator.OneOf(values...)
}

func intRangeValidator(min, max int64) validator.Int64 {
	return int64validator.Between(min, max)
}

func regexValidator(re *regexp.Regexp, what string) validator.String {
	return &reValidator{re: re, what: what}
}

type reValidator struct {
	re   *regexp.Regexp
	what string
}

func (v *reValidator) Description(_ context.Context) string {
	return fmt.Sprintf("value must be a valid %s", v.what)
}

func (v *reValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v *reValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if !v.re.MatchString(req.ConfigValue.ValueString()) {
		resp.Diagnostics.AddError(
			"Invalid value",
			fmt.Sprintf("value %q is not a valid %s", req.ConfigValue.ValueString(), v.what),
		)
	}
}
