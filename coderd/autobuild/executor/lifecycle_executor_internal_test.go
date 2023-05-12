package executor

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/google/uuid"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/coderd/database/dbfake"
)

func Test_compatibleParameters(t *testing.T) {
	t.Parallel()

	// some helper assertions
	ok := func(req *require.Assertions, err error) {
		req.NoError(err)
	}
	incompatible := func(req *require.Assertions, err error) {
		var inc errIncompatibleParameters
		req.ErrorAs(err, &inc)
	}

	cases := []struct {
		name string
		// rich params from previous build
		lastBuildParameters []database.WorkspaceBuildParameter
		// regular params from previous build
		lastParameterValues []database.ParameterValue
		// regular params on the new template
		newSchemas []database.ParameterSchema
		// rich params on the new template
		newTemplateVersionParameters []database.TemplateVersionParameter
		// expected result
		expected func(req *require.Assertions, err error)
	}{
		{
			name:     "AllEmpty",
			expected: ok,
		},
		{
			name: "OldMatchesNew",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
				{Name: "2"},
			},
			lastParameterValues: []database.ParameterValue{
				{Name: "3"},
				{Name: "4"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
				{Name: "2", Required: true},
			},
			newSchemas: []database.ParameterSchema{
				{Name: "3", AllowOverrideSource: true},
				{Name: "4", AllowOverrideSource: true},
			},
			expected: ok,
		},
		{
			name: "NewSubsetOfOld",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
				{Name: "2"},
			},
			lastParameterValues: []database.ParameterValue{
				{Name: "3"},
				{Name: "4"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
			},
			newSchemas: []database.ParameterSchema{
				{Name: "4", AllowOverrideSource: true},
			},
			expected: ok,
		},
		{
			name: "ChangeParamType",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
				{Name: "2"},
			},
			lastParameterValues: []database.ParameterValue{
				{Name: "3"},
				{Name: "4"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
				{Name: "2", Required: true},
				{Name: "3", Required: true},
			},
			newSchemas: []database.ParameterSchema{
				{Name: "4", AllowOverrideSource: true},
			},
			expected: incompatible,
		},
		{
			name: "OldMissingOverridableSchema",
			lastParameterValues: []database.ParameterValue{
				{Name: "4"},
			},
			newSchemas: []database.ParameterSchema{
				{Name: "3", AllowOverrideSource: true},
				{Name: "4", AllowOverrideSource: true},
			},
			expected: incompatible,
		},
		{
			name: "OldMissingNonOverridableSchema",
			lastParameterValues: []database.ParameterValue{
				{Name: "4"},
			},
			newSchemas: []database.ParameterSchema{
				{Name: "3", AllowOverrideSource: false},
				{Name: "4", AllowOverrideSource: true},
			},
			expected: ok,
		},
		{
			name: "OldMissingRequiredRichParam",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
				{Name: "2", Required: true},
			},
			expected: incompatible,
		},
		{
			name: "OldMissingImmutableRichParam",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
				{Name: "2", Required: false, Mutable: false},
			},
			expected: incompatible,
		},
		{
			name: "OldMissingMutableOptionalRichParam",
			lastBuildParameters: []database.WorkspaceBuildParameter{
				{Name: "1"},
			},
			newTemplateVersionParameters: []database.TemplateVersionParameter{
				{Name: "1", Required: true},
				{Name: "2", Required: false, Mutable: true},
			},
			expected: ok,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			req := require.New(t)
			fakeDB := dbfake.New()
			templateID := uuid.New()
			version, err := fakeDB.InsertTemplateVersion(ctx, database.InsertTemplateVersionParams{
				ID:         uuid.New(),
				TemplateID: uuid.NullUUID{UUID: templateID, Valid: true},
				JobID:      uuid.New(),
			})
			req.NoError(err)
			template, err := fakeDB.InsertTemplate(ctx, database.InsertTemplateParams{
				ID:              uuid.New(),
				ActiveVersionID: version.ID,
				Provisioner:     database.ProvisionerTypeEcho,
			})
			req.NoError(err)

			workspace, err := fakeDB.InsertWorkspace(ctx, database.InsertWorkspaceParams{
				ID:               uuid.New(),
				TemplateID:       uuid.UUID{},
				AutomaticUpdates: database.AutomaticUpdatesAlways,
			})
			req.NoError(err)

			for _, s := range tc.newSchemas {
				_, err = fakeDB.InsertParameterSchema(ctx, database.InsertParameterSchemaParams{
					ID:                       uuid.New(),
					JobID:                    version.JobID,
					Name:                     s.Name,
					AllowOverrideSource:      s.AllowOverrideSource,
					DefaultSourceScheme:      database.ParameterSourceSchemeData,
					DefaultDestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
					ValidationTypeSystem:     database.ParameterTypeSystemHCL,
				})
				req.NoError(err)
			}
			for _, p := range tc.newTemplateVersionParameters {
				_, err := fakeDB.InsertTemplateVersionParameter(ctx, database.InsertTemplateVersionParameterParams{
					TemplateVersionID: version.ID,
					Name:              p.Name,
					Mutable:           p.Mutable,
					Required:          p.Required,
				})
				req.NoError(err)
			}
			for _, v := range tc.lastParameterValues {
				_, err := fakeDB.InsertParameterValue(ctx, database.InsertParameterValueParams{
					ID:                uuid.New(),
					Name:              v.Name,
					Scope:             database.ParameterScopeWorkspace,
					ScopeID:           workspace.ID,
					SourceScheme:      database.ParameterSourceSchemeData,
					DestinationScheme: database.ParameterDestinationSchemeProvisionerVariable,
				})
				req.NoError(err)
			}

			err = compatibleParameters(ctx, fakeDB, workspace, template, tc.lastBuildParameters)
			tc.expected(req, err)
		})
	}

}
