package apigimport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gen-openapi/internal/config"

	apigmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2/model"
)

type fakeImporter struct {
	request *apigmodel.ImportApiDefinitionsV2Request
	resp    *apigmodel.ImportApiDefinitionsV2Response
}

func (f *fakeImporter) ImportApiDefinitionsV2(req *apigmodel.ImportApiDefinitionsV2Request) (*apigmodel.ImportApiDefinitionsV2Response, error) {
	f.request = req
	return f.resp, nil
}

func TestImportWithClientBuildsRequest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "openAPI.yaml")
	if err := os.WriteFile(file, []byte("openapi: 3.0.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	groupID := "group-1"
	path := "/v1/pets"
	method := "GET"
	action := apigmodel.GetSuccessActionEnum().CREATE
	id := "api-1"
	client := &fakeImporter{resp: &apigmodel.ImportApiDefinitionsV2Response{
		GroupId: &groupID,
		Success: &[]apigmodel.Success{{
			Path:   &path,
			Method: &method,
			Action: &action,
			Id:     &id,
		}},
	}}

	result, err := importWithClient(context.Background(), client, Options{
		FilePath: file,
		Target: config.ImportTarget{
			Region:     "cn-north-4",
			ProjectID:  "project-1",
			InstanceID: "instance-1",
			GroupID:    "group-1",
			APIMode:    "merge",
			ExtendMode: "merge",
		},
	})
	if err != nil {
		t.Fatalf("importWithClient: %v", err)
	}
	if client.request == nil {
		t.Fatal("expected SDK request")
	}
	if client.request.InstanceId != "instance-1" {
		t.Fatalf("instance id = %q", client.request.InstanceId)
	}
	if client.request.Body == nil || client.request.Body.FileName == nil || client.request.Body.GroupId == nil {
		t.Fatalf("expected multipart body with file and group id: %+v", client.request.Body)
	}
	if result.GroupID != "group-1" || len(result.Success) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Success[0].Action != "create" {
		t.Fatalf("action = %q", result.Success[0].Action)
	}
}

func TestValidateOptionsRequiresCredentials(t *testing.T) {
	err := validateOptions(Options{FilePath: "openAPI.yaml", Target: config.ImportTarget{
		Region:     "cn-north-4",
		ProjectID:  "project-1",
		InstanceID: "instance-1",
		GroupID:    "group-1",
		APIMode:    "merge",
		ExtendMode: "merge",
	}})
	if err == nil {
		t.Fatal("expected missing credentials error")
	}
}
