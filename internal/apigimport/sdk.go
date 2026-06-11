package apigimport

import (
	"context"
	"fmt"
	"os"

	"gen-openapi/internal/config"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	httpconfig "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/def"
	apig "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2"
	apigmodel "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2/model"
	apigregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/apig/v2/region"
)

type Options struct {
	FilePath string
	Target   config.ImportTarget
	AK       string
	SK       string
}

type Result struct {
	GroupID string
	Success []ImportedAPI
	Failure []ImportFailure
	Ignore  []IgnoredAPI
	Swagger string
}

type ImportedAPI struct {
	Path   string
	Method string
	Action string
	ID     string
}

type ImportFailure struct {
	Path      string
	Method    string
	ErrorCode string
	ErrorMsg  string
}

type IgnoredAPI struct {
	Path   string
	Method string
}

type importer interface {
	ImportApiDefinitionsV2(*apigmodel.ImportApiDefinitionsV2Request) (*apigmodel.ImportApiDefinitionsV2Response, error)
}

func Import(ctx context.Context, opts Options) (*Result, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}
	client, err := newSDKClient(opts)
	if err != nil {
		return nil, err
	}
	return importWithClient(ctx, client, opts)
}

func validateOptions(opts Options) error {
	if opts.FilePath == "" {
		return fmt.Errorf("apig import file path is required")
	}
	if opts.AK == "" {
		return fmt.Errorf("HUAWEICLOUD_SDK_AK is required")
	}
	if opts.SK == "" {
		return fmt.Errorf("HUAWEICLOUD_SDK_SK is required")
	}
	if err := config.ValidateImportTarget(&opts.Target); err != nil {
		return err
	}
	return nil
}

func newSDKClient(opts Options) (*apig.ApigClient, error) {
	region, err := apigregion.SafeValueOf(opts.Target.Region)
	if err != nil {
		return nil, fmt.Errorf("resolve apig region %s: %w", opts.Target.Region, err)
	}
	cred, err := basic.NewCredentialsBuilder().
		WithAk(opts.AK).
		WithSk(opts.SK).
		WithProjectId(opts.Target.ProjectID).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("build huawei credentials: %w", err)
	}
	hcClient, err := apig.ApigClientBuilder().
		WithRegion(region).
		WithCredential(cred).
		WithHttpConfig(httpconfig.DefaultHttpConfig()).
		SafeBuild()
	if err != nil {
		return nil, fmt.Errorf("build apig client: %w", err)
	}
	return apig.NewApigClient(hcClient), nil
}

func importWithClient(ctx context.Context, client importer, opts Options) (*Result, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	file, err := os.Open(opts.FilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	body := &apigmodel.ImportApiDefinitionsV2RequestBody{
		IsCreateGroup: def.NewMultiPart(opts.Target.IsCreateGroup),
		ExtendMode:    def.NewMultiPart(opts.Target.ExtendMode),
		SimpleMode:    def.NewMultiPart(opts.Target.SimpleMode),
		MockMode:      def.NewMultiPart(opts.Target.MockMode),
		ApiMode:       def.NewMultiPart(opts.Target.APIMode),
		FileName:      def.NewFilePart(file),
	}
	if !opts.Target.IsCreateGroup {
		body.GroupId = def.NewMultiPart(opts.Target.GroupID)
	}

	resp, err := client.ImportApiDefinitionsV2(&apigmodel.ImportApiDefinitionsV2Request{
		InstanceId: opts.Target.InstanceID,
		Body:       body,
	})
	if err != nil {
		return nil, err
	}
	return normalizeResponse(resp), nil
}

func normalizeResponse(resp *apigmodel.ImportApiDefinitionsV2Response) *Result {
	result := &Result{}
	if resp == nil {
		return result
	}
	result.GroupID = str(resp.GroupId)
	if resp.Swagger != nil {
		result.Swagger = str(resp.Swagger.Result)
	}
	if resp.Success != nil {
		for _, item := range *resp.Success {
			result.Success = append(result.Success, ImportedAPI{
				Path:   str(item.Path),
				Method: str(item.Method),
				Action: actionString(item.Action),
				ID:     str(item.Id),
			})
		}
	}
	if resp.Failure != nil {
		for _, item := range *resp.Failure {
			result.Failure = append(result.Failure, ImportFailure{
				Path:      str(item.Path),
				Method:    str(item.Method),
				ErrorCode: str(item.ErrorCode),
				ErrorMsg:  str(item.ErrorMsg),
			})
		}
	}
	if resp.Ignore != nil {
		for _, item := range *resp.Ignore {
			result.Ignore = append(result.Ignore, IgnoredAPI{
				Path:   str(item.Path),
				Method: str(item.Method),
			})
		}
	}
	return result
}

func str(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func actionString(v *apigmodel.SuccessAction) string {
	if v == nil {
		return ""
	}
	return v.Value()
}
