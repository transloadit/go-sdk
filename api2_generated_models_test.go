package transloadit

// This file is generated from Transloadit API2 contracts. If it looks wrong,
// please report the issue instead of editing this file by hand; the source fix
// belongs in the contract generator so all SDKs stay in sync.

import (
	"reflect"
	"strings"
	"testing"
)

func TestGeneratedApi2ContractModelFields(t *testing.T) {
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "AssemblyID", "assembly_id", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "AssemblySSLURL", "assembly_ssl_url", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "AssemblyURL", "assembly_url", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "Error", "error", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "Ok", "ok", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "Results", "results", reflect.TypeOf((*map[string][]*FileInfo)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "TUSURL", "tus_url", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(AssemblyInfo{}), "Uploads", "uploads", reflect.TypeOf((*[]*FileInfo)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(FileInfo{}), "Field", "field", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(FileInfo{}), "IsTUSFile", "is_tus_file", reflect.TypeOf((*bool)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(FileInfo{}), "Name", "name", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(FileInfo{}), "TUSUploadURL", "tus_upload_url", reflect.TypeOf((*string)(nil)).Elem())
	assertGeneratedApi2ContractModelField(t, reflect.TypeOf(FileInfo{}), "UserMeta", "user_meta", reflect.TypeOf((*map[string]interface{})(nil)).Elem())
}

func assertGeneratedApi2ContractModelField(t *testing.T, modelType reflect.Type, fieldName string, jsonField string, expectedType reflect.Type) {
	t.Helper()

	field, ok := modelType.FieldByName(fieldName)
	if !ok {
		t.Fatalf("%s.%s is missing", modelType.Name(), fieldName)
	}
	if field.Type != expectedType {
		t.Fatalf("%s.%s has type %s, expected %s", modelType.Name(), fieldName, field.Type, expectedType)
	}

	jsonTag := field.Tag.Get("json")
	if jsonTag != jsonField && !strings.HasPrefix(jsonTag, jsonField+",") {
		t.Fatalf("%s.%s has json tag %q, expected %q", modelType.Name(), fieldName, jsonTag, jsonField)
	}
}
