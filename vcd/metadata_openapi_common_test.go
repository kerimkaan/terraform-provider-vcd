//go:build rde || functional || ALL

package vcd

import (
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
	"strconv"
	"strings"
	"testing"
)

// testOpenApiMetadataEntryCRUD executes a test that asserts CRUD operation behaviours of "metadata_entry" attribute in the given HCL
// templates, that must correspond to a resource and a data source referencing this resource.
// The HCL template requires {{.Name}} and {{.Metadata}} fields, and the usual {{.Org}} and {{.Vdc}}.
// You can add extra parameters as well to inject in the given HCL template, or override these mentioned ones.
// The data source HCL is always concatenated to the resource after creation, and it's skipped on binary tests.
func testOpenApiMetadataEntryCRUD(t *testing.T, resourceTemplate, resourceAddress, datasourceTemplate, datasourceAddress string, extraParams StringMap) {
	preTestChecks(t)
	var params = StringMap{
		"Org":  testConfig.VCD.Org,
		"Vdc":  testConfig.Nsxt.Vdc,
		"Name": t.Name(),
	}

	// This output allows to perform some assertions on the ID inside a TypeSet,
	// which is impossible to obtain otherwise.
	outputHcl := `
output "metadata_id" {
  value = tolist(` + resourceAddress + `.metadata_entry)[0].id
}
`
	templateWithOutput := resourceTemplate + outputHcl

	for extraParam, extraParamValue := range extraParams {
		params[extraParam] = extraParamValue
	}
	testParamsNotEmpty(t, params)

	params["FuncName"] = t.Name() + "NoMetadata"
	params["Metadata"] = " "
	noMetadataHcl := templateFill(resourceTemplate, params)
	debugPrintf("#[DEBUG] CONFIGURATION NoMetadata: %s", noMetadataHcl)

	params["FuncName"] = t.Name() + "Create"
	params["Metadata"] = getOpenApiMetadataTestingHcl(1, 1, 1, 1, 2, 1, 1)
	createHcl := templateFill(templateWithOutput, params)
	debugPrintf("#[DEBUG] CONFIGURATION Create: %s", createHcl)

	params["FuncName"] = t.Name() + "WithDatasource"
	withDatasourceHcl := templateFill(datasourceTemplate+"\n# skip-binary-test\n"+templateWithOutput, params)
	debugPrintf("#[DEBUG] CONFIGURATION WithDatasource: %s", withDatasourceHcl)

	params["FuncName"] = t.Name() + "DeleteOneKey"
	params["Metadata"] = getOpenApiMetadataTestingHcl(1, 1, 0, 1, 2, 1, 1)
	deleteOneKeyHcl := templateFill(templateWithOutput, params)
	debugPrintf("#[DEBUG] CONFIGURATION DeleteOneKey: %s", deleteOneKeyHcl)

	params["FuncName"] = t.Name() + "Update"
	params["Metadata"] = strings.NewReplacer("stringValue", "stringValueUpdated").Replace(params["Metadata"].(string))
	updateHcl := templateFill(templateWithOutput, params)
	debugPrintf("#[DEBUG] CONFIGURATION Update: %s", updateHcl)

	params["FuncName"] = t.Name() + "UpdateForceRecreate"
	params["Metadata"] = strings.NewReplacer("NumberEntry", "StringEntry").Replace(params["Metadata"].(string))
	updateForceRecreateHcl := templateFill(templateWithOutput, params)
	debugPrintf("#[DEBUG] CONFIGURATION UpdateForceRecreate: %s", updateForceRecreateHcl)

	params["FuncName"] = t.Name() + "Delete"
	params["Metadata"] = " "
	deleteHcl := templateFill(resourceTemplate, params)
	debugPrintf("#[DEBUG] CONFIGURATION Delete: %s", deleteHcl)

	params["FuncName"] = t.Name() + "WithDefaults"
	params["Metadata"] = "metadata_entry {\n\tkey = \"defaultKey\"\nvalue = \"defaultValue\"\n}"
	withDefaults := templateFill(resourceTemplate, params)
	debugPrintf("#[DEBUG] CONFIGURATION WithDefaults: %s", withDefaults)

	if vcdShortTest {
		t.Skip(acceptanceTestsSkipped)
		return
	}

	// This is used to validate that metadata IDs don't change despite of an update/delete.
	cachedId := testCachedFieldValue{}
	testIdsDontChange := func() func(s *terraform.State) error {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Outputs["metadata_id"]
			if !ok {
				return fmt.Errorf("output 'metadata_id' not found")
			}

			value := rs.String()
			if cachedId.fieldValue != value || value == "" {
				return fmt.Errorf("expected metadata_id to be '%s' but it changed to '%s'", cachedId.fieldValue, value)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: noMetadataHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "0"),
				),
			},
			{
				Config: createHcl,
				Taint:  []string{resourceAddress}, // Forces re-creation to test Create with metadata.
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						rs, ok := s.RootModule().Outputs["metadata_id"]
						if !ok {
							return fmt.Errorf("output 'metadata_id' not found")
						}
						cachedId.fieldValue = rs.String()
						return nil
					},
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "8"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "stringKey1", "stringValue1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "numberKey1", "1", types.OpenApiMetadataNumberEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "boolKey1", "false", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "readOnly1", "readOnly1", types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace1", types.OpenApiMetadataStringEntry, "TENANT", "namespace1", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace2", types.OpenApiMetadataStringEntry, "TENANT", "namespace2", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "provider1", "provider1", types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "persistent1", "persistent1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true"),
				),
			},
			{
				Config: withDatasourceHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrPair(datasourceAddress, "id", resourceAddress, "id"),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "8"),
					resource.TestCheckResourceAttr(datasourceAddress, "metadata_entry.#", "8"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "stringKey1", "stringValue1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "numberKey1", "1", types.OpenApiMetadataNumberEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "boolKey1", "false", types.OpenApiMetadataBooleanEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "readOnly1", "readOnly1", types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace1", types.OpenApiMetadataStringEntry, "TENANT", "namespace1", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace2", types.OpenApiMetadataStringEntry, "TENANT", "namespace2", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "provider1", "provider1", types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "persistent1", "persistent1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true"),
				),
			},
			{
				Config: deleteOneKeyHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "7"),
					// The bool is deleted
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "stringKey1", "stringValue1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "numberKey1", "1", types.OpenApiMetadataNumberEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "readOnly1", "readOnly1", types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace1", types.OpenApiMetadataStringEntry, "TENANT", "namespace1", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace2", types.OpenApiMetadataStringEntry, "TENANT", "namespace2", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "provider1", "provider1", types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "persistent1", "persistent1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true"),
					testIdsDontChange(),
				),
			},
			{
				Config: updateHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "7"),
					// Updated value:
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "stringKey1", "stringValueUpdated1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					// Not updated values:
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "numberKey1", "1", types.OpenApiMetadataNumberEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "readOnly1", "readOnly1", types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace1", types.OpenApiMetadataStringEntry, "TENANT", "namespace1", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace2", types.OpenApiMetadataStringEntry, "TENANT", "namespace2", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "provider1", "provider1", types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "persistent1", "persistent1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true"),
					testIdsDontChange(),
				),
			},
			{
				Config: updateForceRecreateHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "7"),
					// Updated value, from number to string, should force a recreation of the entry:
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "numberKey1", "1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					// Not updated values:
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "stringKey1", "stringValueUpdated1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "readOnly1", "readOnly1", types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace1", types.OpenApiMetadataStringEntry, "TENANT", "namespace1", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "namespace", "namespace2", types.OpenApiMetadataStringEntry, "TENANT", "namespace2", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "provider1", "provider1", types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "persistent1", "persistent1", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true"),
				),
			},
			{
				Config: deleteHcl,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "0"),
				),
			},
			{
				Config: withDefaults,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceAddress, "name", t.Name()),
					resource.TestCheckResourceAttr(resourceAddress, "metadata_entry.#", "1"),
					testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, "defaultKey", "defaultValue", types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false"),
				),
			},
		},
	})
	postTestChecks(t)
}

// getOpenApiMetadataTestingHcl gets valid metadata entries to inject them into an HCL for testing
func getOpenApiMetadataTestingHcl(stringEntries, numberEntries, boolEntries, readonlyEntries, namespacedEntries, providerEntries, persistentEntries int) string {
	hcl := ""
	for i := 1; i <= stringEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("stringKey%d", i), fmt.Sprintf("stringValue%d", i), types.OpenApiMetadataStringEntry, "TENANT", "", "false", "false")
	}
	for i := 1; i <= numberEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("numberKey%d", i), fmt.Sprintf("%d", i), types.OpenApiMetadataNumberEntry, "TENANT", "", "false", "false")
	}
	for i := 1; i <= boolEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("boolKey%d", i), strconv.FormatBool(i%2 == 0), types.OpenApiMetadataBooleanEntry, "TENANT", "", "false", "false")
	}
	for i := 1; i <= readonlyEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("readOnly%d", i), fmt.Sprintf("readOnly%d", i), types.OpenApiMetadataStringEntry, "TENANT", "", "true", "false")
	}
	for i := 1; i <= namespacedEntries; i++ {
		// This one shares the same key, as it is the goal of namespaces
		hcl += getOpenApiMetadataEntryHcl("namespace", fmt.Sprintf("namespace%d", i), types.OpenApiMetadataStringEntry, "TENANT", fmt.Sprintf("namespace%d", i), "false", "false")
	}
	for i := 1; i <= providerEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("provider%d", i), fmt.Sprintf("provider%d", i), types.OpenApiMetadataStringEntry, "PROVIDER", "", "false", "false")
	}
	for i := 1; i <= persistentEntries; i++ {
		hcl += getOpenApiMetadataEntryHcl(fmt.Sprintf("persistent%d", i), fmt.Sprintf("persistent%d", i), types.OpenApiMetadataStringEntry, "TENANT", "", "false", "true")
	}
	return hcl

}

func getOpenApiMetadataEntryHcl(key, value, typedValue, domain, namespace, readonly, persistent string) string {
	hclNamespace := ""
	if namespace != "" {
		hclNamespace = `namespace  = "` + namespace + `"`
	}

	return `
  metadata_entry {
    key        = "` + key + `"
    value      = "` + value + `"
	type       = "` + typedValue + `"
	domain     = "` + domain + `"
    ` + hclNamespace + `
	readonly   = ` + readonly + `
    persistent = ` + persistent + `
  }`
}

// testCheckOpenApiMetadataEntrySetElemNestedAttrs asserts that a given metadata_entry has the expected input for the given resourceAddress.
func testCheckOpenApiMetadataEntrySetElemNestedAttrs(resourceAddress, expectedKey, expectedValue, expectedType, expectedDomain, expectedNamespace, expectedReadonly, expectedPersistent string) resource.TestCheckFunc {
	return resource.TestCheckTypeSetElemNestedAttrs(resourceAddress, "metadata_entry.*",
		map[string]string{
			"key":        expectedKey,
			"value":      expectedValue,
			"type":       expectedType,
			"domain":     expectedDomain,
			"readonly":   expectedReadonly,
			"namespace":  expectedNamespace,
			"persistent": expectedPersistent,
		},
	)
}
