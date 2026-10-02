package models

import "testing"

// A required field is a statement about the buyer's form, not about the shop
// saving the product. Validating a schema with no answers threw 「请填写机房区域」
// at the shop owner and made any product with a mandatory field impossible to
// create or edit.
func TestSchemaValidationDoesNotDemandAnswers(t *testing.T) {
	fields := []ProductFormField{
		{Key: "region", Label: "机房区域", Type: "select", Required: true, Options: []string{"洛杉矶", "圣何塞"}},
	}

	if _, err := ValidateProductFormValues(fields, map[string]string{}, false); err != nil {
		t.Fatalf("saving a schema with a required field was refused: %v", err)
	}

	// The buyer's side still enforces it.
	if _, err := ValidateProductForm(fields, map[string]string{}); err == nil {
		t.Fatal("checkout accepted a missing required answer")
	}
	if _, err := ValidateProductForm(fields, map[string]string{"region": "洛杉矶"}); err != nil {
		t.Fatalf("checkout refused a valid answer: %v", err)
	}
}

// A schema that is broken on its own terms is still refused, whatever the flag:
// an option list that cannot resolve is a mistake however it is read.
func TestSchemaValidationStillChecksTheSchema(t *testing.T) {
	cases := map[string][]ProductFormField{
		"no key":            {{Key: "", Label: "机房区域", Type: "text"}},
		"no label":          {{Key: "region", Label: "", Type: "text"}},
		"unknown type":      {{Key: "region", Label: "机房区域", Type: "date"}},
		"select no options": {{Key: "region", Label: "机房区域", Type: "select"}},
		"duplicate key": {
			{Key: "region", Label: "机房区域", Type: "text"},
			{Key: "region", Label: "备用区域", Type: "text"},
		},
	}
	for name, fields := range cases {
		if _, err := ValidateProductFormValues(fields, map[string]string{}, false); err == nil {
			t.Errorf("%s: an invalid schema was accepted", name)
		}
	}
}
