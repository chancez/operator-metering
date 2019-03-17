package reporting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	metering "github.com/operator-framework/operator-metering/pkg/apis/metering/v1alpha1"
	"github.com/operator-framework/operator-metering/pkg/operator/reportingutil"
	"github.com/operator-framework/operator-metering/test/testhelpers"
)

func TestValidateGenerationQueryDependencies(t *testing.T) {
	// reportQuery := testhelpers.NewReportGenerationQuery("uninitialized-query", "default", nil)
	dataSourceTableUnset := testhelpers.NewReportDataSource("uninitialized-datasource", "default")
	dataSourceTableSet := testhelpers.NewReportDataSource("initialized-datasource", "default")
	dataSourceTableSet.Status.TableRef.Name = reportingutil.DataSourceTableName("test-ns", "initialized-datasource")

	reportTableUnset := testhelpers.NewReport("uninitialized-report", "default", "some-query", nil, nil, metering.ReportStatus{})
	reportTableSet := testhelpers.NewReport("initialized-report", "default", "some-query", nil, nil, metering.ReportStatus{
		TableRef: v1.LocalObjectReference{reportingutil.ReportTableName("test-ns", "initialized-report")},
	})

	// initializedQueries := []*metering.ReportGenerationQuery{
	// 	reportQuery,
	// }
	uninitializedDataSources := []*metering.ReportDataSource{
		dataSourceTableUnset,
	}
	initializedDataSources := []*metering.ReportDataSource{
		dataSourceTableSet,
	}
	uninitializedReports := []*metering.Report{
		reportTableUnset,
	}
	initializedReports := []*metering.Report{
		reportTableSet,
	}

	tests := map[string]struct {
		deps      ReportGenerationQueryDependencies
		expectErr bool
	}{
		"no dependencies results in no errors": {
			deps: ReportGenerationQueryDependencies{},
		},
		"ReportGenerationQueryDependencies dependencies on other queries is valid": {
			deps: ReportGenerationQueryDependencies{
				// ReportGenerationQueries: initializedQueries,
			},
		},
		"ReportDataSource dependencies with status.tableRef.name unset is a validation error": {
			deps: ReportGenerationQueryDependencies{
				ReportDataSources: uninitializedDataSources,
			},
			expectErr: true,
		},
		"ReportDataSource dependencies with status.tableRef.name set is valid": {
			deps: ReportGenerationQueryDependencies{
				ReportDataSources: initializedDataSources,
			},
		},
		"Report dependencies with status.tableRef.name unset is a validation error": {
			deps: ReportGenerationQueryDependencies{
				Reports: uninitializedReports,
			},
			expectErr: true,
		},
		"Report dependencies with status.tableRef.name set is valid": {
			deps: ReportGenerationQueryDependencies{
				Reports: initializedReports,
			},
		},
		"mixing valid and invalid dependencies is a validation error": {
			deps: ReportGenerationQueryDependencies{
				Reports: uninitializedReports,
				// ReportGenerationQueries: initializedQueries,
				ReportDataSources: uninitializedDataSources,
			},
			expectErr: true,
		},
		"mixing valid dependencies is a valid": {
			deps: ReportGenerationQueryDependencies{
				Reports: uninitializedReports,
				// ReportGenerationQueries: initializedQueries,
				ReportDataSources: initializedDataSources,
			},
			expectErr: true,
		},
	}

	for testName, tt := range tests {
		testName := testName
		tt := tt
		t.Run(testName, func(t *testing.T) {
			err := ValidateGenerationQueryDependencies(&tt.deps, nil)
			if tt.expectErr {
				assert.NotNil(t, err, "expected a validation error")
			} else {
				assert.NoError(t, err, "expected validation to pass")
			}
		})
	}
}

func TestGetGenerationQueryDependencies(t *testing.T) {
	testNs := "test-ns"

	ds1 := testhelpers.NewReportDataSource("datasource1", testNs)
	ds2 := testhelpers.NewReportDataSource("datasource2", testNs)
	ds3 := testhelpers.NewReportDataSource("datasource3", testNs)
	ds4 := testhelpers.NewReportDataSource("datasource4", testNs)
	ds5 := testhelpers.NewReportDataSource("datasource5", testNs)
	ds6 := testhelpers.NewReportDataSource("datasource6", testNs)

	testQuery := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "test-query",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{
			DataSources: []string{
				"datasource1",
				"datasource2",
			},
			DynamicReportQueries: []string{
				"dynamicquery1",
				"dynamicquery2",
			},
		},
	}

	query1 := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "query1",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{
			DataSources: []string{
				"datasource3",
			},
		},
	}

	dynamicquery1 := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "dynamicquery1",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{
			DataSources: []string{
				"datasource4",
			},
		},
	}

	query2 := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "query2",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{
			DataSources: []string{
				"datasource5",
			},
		},
	}

	dynamicquery2 := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "dynamicquery2",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{},
	}

	query3 := &metering.ReportGenerationQuery{
		ObjectMeta: meta.ObjectMeta{
			Name:      "query3",
			Namespace: testNs,
		},
		Spec: metering.ReportGenerationQuerySpec{
			DataSources: []string{
				"datasource6",
			},
		},
	}

	expectedDeps := &ReportGenerationQueryDependencies{
		ReportDataSources: []*metering.ReportDataSource{
			ds1, ds2, ds4,
			// ds3, ds4, ds5, ds6,
		},
		// ReportGenerationQueries: []*metering.ReportGenerationQuery{
		// 	query1, query2, query3,
		// },
		DynamicReportGenerationQueries: []*metering.ReportGenerationQuery{
			dynamicquery1, dynamicquery2,
		},
		Reports: []*metering.Report{},
	}

	dataSourceStore := map[string]*metering.ReportDataSource{
		"test-ns/datasource1": ds1,
		"test-ns/datasource2": ds2,
		"test-ns/datasource3": ds3,
		"test-ns/datasource4": ds4,
		"test-ns/datasource5": ds5,
		"test-ns/datasource6": ds6,
	}
	queryStore := map[string]*metering.ReportGenerationQuery{
		"test-ns/test-query":    testQuery,
		"test-ns/query1":        query1,
		"test-ns/query2":        query2,
		"test-ns/query3":        query3,
		"test-ns/dynamicquery1": dynamicquery1,
		"test-ns/dynamicquery2": dynamicquery2,
	}
	reportStore := make(map[string]*metering.Report)

	queryGetter := reportGenerationQueryGetterFunc(func(namespace, name string) (*metering.ReportGenerationQuery, error) {
		query, ok := queryStore[namespace+"/"+name]
		if ok {
			return query, nil
		} else {
			return nil, errors.NewNotFound(metering.Resource("ReportGenerationQuery"), name)
		}
	})
	dataSourceGetter := reportDataSourceGetterFunc(func(namespace, name string) (*metering.ReportDataSource, error) {
		dataSource, ok := dataSourceStore[namespace+"/"+name]
		if ok {
			return dataSource, nil
		} else {
			return nil, errors.NewNotFound(metering.Resource("ReportDataSource"), name)
		}
	})
	reportGetter := reportGetterFunc(func(namespace, name string) (*metering.Report, error) {
		report, ok := reportStore[namespace+"/"+name]
		if ok {
			return report, nil
		} else {
			return nil, errors.NewNotFound(metering.Resource("Report"), name)
		}
	})

	deps, err := GetGenerationQueryDependencies(
		queryGetter,
		dataSourceGetter,
		reportGetter,
		testQuery,
	)
	require.NoError(t, err)
	require.Equal(t, expectedDeps, deps)
}
