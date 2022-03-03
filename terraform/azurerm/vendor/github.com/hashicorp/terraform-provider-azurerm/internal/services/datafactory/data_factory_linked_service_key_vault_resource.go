package datafactory

import (
	"fmt"
	"log"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/datafactory/mgmt/2018-06-01/datafactory"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/azure"
	"github.com/hashicorp/terraform-provider-azurerm/helpers/tf"
	"github.com/hashicorp/terraform-provider-azurerm/internal/clients"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/datafactory/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/datafactory/validate"
	keyVaultParse "github.com/hashicorp/terraform-provider-azurerm/internal/services/keyvault/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/internal/timeouts"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

func resourceDataFactoryLinkedServiceKeyVault() *pluginsdk.Resource {
	return &pluginsdk.Resource{
		Create: resourceDataFactoryLinkedServiceKeyVaultCreateUpdate,
		Read:   resourceDataFactoryLinkedServiceKeyVaultRead,
		Update: resourceDataFactoryLinkedServiceKeyVaultCreateUpdate,
		Delete: resourceDataFactoryLinkedServiceKeyVaultDelete,

		Importer: pluginsdk.ImporterValidatingResourceIdThen(func(id string) error {
			_, err := parse.LinkedServiceID(id)
			return err
		}, importDataFactoryLinkedService(datafactory.TypeBasicLinkedServiceTypeAzureKeyVault)),

		Timeouts: &pluginsdk.ResourceTimeout{
			Create: pluginsdk.DefaultTimeout(30 * time.Minute),
			Read:   pluginsdk.DefaultTimeout(5 * time.Minute),
			Update: pluginsdk.DefaultTimeout(30 * time.Minute),
			Delete: pluginsdk.DefaultTimeout(30 * time.Minute),
		},

		Schema: map[string]*pluginsdk.Schema{
			"name": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validate.LinkedServiceDatasetName,
			},

			// TODO remove in 3.0
			"data_factory_name": {
				Type:         pluginsdk.TypeString,
				Optional:     true,
				Computed:     true,
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryName(),
				Deprecated:   "`data_factory_name` is deprecated in favour of `data_factory_id` and will be removed in version 3.0 of the AzureRM provider",
				ExactlyOneOf: []string{"data_factory_id"},
			},

			"data_factory_id": {
				Type:         pluginsdk.TypeString,
				Optional:     true, // TODO set to Required in 3.0
				Computed:     true, // TODO remove in 3.0
				ForceNew:     true,
				ValidateFunc: validate.DataFactoryID,
				ExactlyOneOf: []string{"data_factory_name"},
			},

			// There's a bug in the Azure API where this is returned in lower-case
			// BUG: https://github.com/Azure/azure-rest-api-specs/issues/5788
			"resource_group_name": azure.SchemaResourceGroupNameDiffSuppress(),

			"key_vault_id": {
				Type:         pluginsdk.TypeString,
				Required:     true,
				ValidateFunc: azure.ValidateResourceID,
			},

			"description": {
				Type:         pluginsdk.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},

			"integration_runtime_name": {
				Type:         pluginsdk.TypeString,
				Optional:     true,
				ValidateFunc: validation.StringIsNotEmpty,
			},

			"parameters": {
				Type:     pluginsdk.TypeMap,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
				},
			},

			"annotations": {
				Type:     pluginsdk.TypeList,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
				},
			},

			"additional_properties": {
				Type:     pluginsdk.TypeMap,
				Optional: true,
				Elem: &pluginsdk.Schema{
					Type: pluginsdk.TypeString,
				},
			},
		},
	}
}

func resourceDataFactoryLinkedServiceKeyVaultCreateUpdate(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.LinkedServiceClient
	subscriptionId := meta.(*clients.Client).DataFactory.LinkedServiceClient.SubscriptionID
	keyVaultsClient := meta.(*clients.Client).KeyVault
	ctx, cancel := timeouts.ForCreateUpdate(meta.(*clients.Client).StopContext, d)
	defer cancel()

	// TODO 3.0: remove/simplify this after deprecation
	var err error
	var dataFactoryId *parse.DataFactoryId
	if v := d.Get("data_factory_name").(string); v != "" {
		newDataFactoryId := parse.NewDataFactoryID(subscriptionId, d.Get("resource_group_name").(string), d.Get("data_factory_name").(string))
		dataFactoryId = &newDataFactoryId
	}
	if v := d.Get("data_factory_id").(string); v != "" {
		dataFactoryId, err = parse.DataFactoryID(v)
		if err != nil {
			return err
		}
	}

	id := parse.NewLinkedServiceID(subscriptionId, dataFactoryId.ResourceGroup, dataFactoryId.FactoryName, d.Get("name").(string))

	keyVaultId, err := keyVaultParse.VaultID(d.Get("key_vault_id").(string))
	if err != nil {
		return err
	}

	keyVaultBaseUri, err := keyVaultsClient.BaseUriForKeyVault(ctx, *keyVaultId)
	if err != nil {
		return err
	}

	if d.IsNewResource() {
		existing, err := client.Get(ctx, id.ResourceGroup, id.FactoryName, id.Name, "")
		if err != nil {
			if !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing Data Factory Key Vault %s: %+v", id, err)
			}
		}

		if existing.ID != nil && *existing.ID != "" {
			return tf.ImportAsExistsError("azurerm_data_factory_linked_service_key_vault", *existing.ID)
		}
	}

	azureKeyVaultProperties := &datafactory.AzureKeyVaultLinkedServiceTypeProperties{
		BaseURL: keyVaultBaseUri,
	}

	azureKeyVaultLinkedService := &datafactory.AzureKeyVaultLinkedService{
		Description:                              utils.String(d.Get("description").(string)),
		AzureKeyVaultLinkedServiceTypeProperties: azureKeyVaultProperties,
		Type:                                     datafactory.TypeBasicLinkedServiceTypeAzureKeyVault,
	}

	if v, ok := d.GetOk("parameters"); ok {
		azureKeyVaultLinkedService.Parameters = expandDataFactoryParameters(v.(map[string]interface{}))
	}

	if v, ok := d.GetOk("integration_runtime_name"); ok {
		azureKeyVaultLinkedService.ConnectVia = expandDataFactoryLinkedServiceIntegrationRuntime(v.(string))
	}

	if v, ok := d.GetOk("additional_properties"); ok {
		azureKeyVaultLinkedService.AdditionalProperties = v.(map[string]interface{})
	}

	if v, ok := d.GetOk("annotations"); ok {
		annotations := v.([]interface{})
		azureKeyVaultLinkedService.Annotations = &annotations
	}

	linkedService := datafactory.LinkedServiceResource{
		Properties: azureKeyVaultLinkedService,
	}

	if _, err := client.CreateOrUpdate(ctx, id.ResourceGroup, id.FactoryName, id.Name, linkedService, ""); err != nil {
		return fmt.Errorf("creating/updating Data Factory Key Vault %s: %+v", id, err)
	}

	d.SetId(id.ID())

	return resourceDataFactoryLinkedServiceKeyVaultRead(d, meta)
}

func resourceDataFactoryLinkedServiceKeyVaultRead(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.LinkedServiceClient
	keyVaultsClient := meta.(*clients.Client).KeyVault
	resourcesClient := meta.(*clients.Client).Resource
	ctx, cancel := timeouts.ForRead(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.LinkedServiceID(d.Id())
	if err != nil {
		return err
	}

	dataFactoryId := parse.NewDataFactoryID(id.SubscriptionId, id.ResourceGroup, id.FactoryName)

	resp, err := client.Get(ctx, id.ResourceGroup, id.FactoryName, id.Name, "")
	if err != nil {
		if utils.ResponseWasNotFound(resp.Response) {
			d.SetId("")
			return nil
		}

		return fmt.Errorf("retrieving Data Factory Key Vault %s: %+v", *id, err)
	}

	d.Set("name", resp.Name)
	d.Set("resource_group_name", id.ResourceGroup)
	// TODO 3.0: remove
	d.Set("data_factory_name", id.FactoryName)
	d.Set("data_factory_id", dataFactoryId.ID())

	keyVault, ok := resp.Properties.AsAzureKeyVaultLinkedService()
	if !ok {
		return fmt.Errorf("classifying Data Factory Key Vault %s: Expected: %q Received: %q", *id, datafactory.TypeBasicLinkedServiceTypeAzureKeyVault, *resp.Type)
	}

	d.Set("additional_properties", keyVault.AdditionalProperties)
	d.Set("description", keyVault.Description)

	annotations := flattenDataFactoryAnnotations(keyVault.Annotations)
	if err := d.Set("annotations", annotations); err != nil {
		return fmt.Errorf("setting `annotations`: %+v", err)
	}

	parameters := flattenDataFactoryParameters(keyVault.Parameters)
	if err := d.Set("parameters", parameters); err != nil {
		return fmt.Errorf("setting `parameters`: %+v", err)
	}

	if connectVia := keyVault.ConnectVia; connectVia != nil {
		if connectVia.ReferenceName != nil {
			d.Set("integration_runtime_name", connectVia.ReferenceName)
		}
	}

	baseUrl := ""
	if properties := keyVault.AzureKeyVaultLinkedServiceTypeProperties; properties != nil {
		if properties.BaseURL != nil {
			val, ok := properties.BaseURL.(string)
			if ok {
				baseUrl = val
			} else {
				log.Printf("[DEBUG] Skipping base url string %q since it's not a string", val)
			}
		}
	}

	var keyVaultId *string
	if baseUrl != "" {
		keyVaultId, err = keyVaultsClient.KeyVaultIDFromBaseUrl(ctx, resourcesClient, baseUrl)
		if err != nil {
			return err
		}
	}
	d.Set("key_vault_id", keyVaultId)

	return nil
}

func resourceDataFactoryLinkedServiceKeyVaultDelete(d *pluginsdk.ResourceData, meta interface{}) error {
	client := meta.(*clients.Client).DataFactory.LinkedServiceClient
	ctx, cancel := timeouts.ForDelete(meta.(*clients.Client).StopContext, d)
	defer cancel()

	id, err := parse.LinkedServiceID(d.Id())
	if err != nil {
		return err
	}

	response, err := client.Delete(ctx, id.ResourceGroup, id.FactoryName, id.Name)
	if err != nil {
		if !utils.ResponseWasNotFound(response) {
			return fmt.Errorf("deleting Data Factory Key Vault %s: %+v", *id, err)
		}
	}

	return nil
}