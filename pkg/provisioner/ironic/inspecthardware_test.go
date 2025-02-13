package ironic

import (
	"net/http"
	"testing"
	"time"

	"github.com/wcecs/baremetal-operator/pkg/bmc"
	"github.com/wcecs/baremetal-operator/pkg/provisioner"
	"github.com/wcecs/baremetal-operator/pkg/provisioner/ironic/clients"
	"github.com/wcecs/baremetal-operator/pkg/provisioner/ironic/testserver"

	metal3v1alpha1 "github.com/wcecs/baremetal-operator/apis/metal3.io/v1alpha1"

	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/baremetalintrospection/v1/introspection"
	"github.com/stretchr/testify/assert"
)

func TestInspectHardware(t *testing.T) {

	nodeUUID := "33ce8659-7400-4c68-9535-d10766f07a58"

	cases := []struct {
		name      string
		ironic    *testserver.IronicMock
		inspector *testserver.InspectorMock

		force bool

		expectedStarted      bool
		expectedDirty        bool
		expectedRequestAfter int
		expectedResultError  string
		expectedDetailsHost  string

		expectedPublish string
		expectedError   string
	}{
		{
			name: "introspection-status-move-from-available",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "available",
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),

			expectedStarted:      false,
			expectedDirty:        true,
			expectedRequestAfter: 10,
		},
		{
			name: "introspection-status-start-new-hardware-inspection",
			ironic: testserver.NewIronic(t).WithDefaultResponses().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "active",
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name:   "introspection-data-failed",
			ironic: testserver.NewIronic(t).WithDefaultResponses(),
			inspector: testserver.NewInspector(t).Ready().
				WithIntrospection(nodeUUID, introspection.Introspection{
					Finished: true,
				}).
				WithIntrospectionDataFailed(nodeUUID, http.StatusBadRequest),

			expectedError: "failed to retrieve hardware introspection data: Bad request with: \\[GET http://127.0.0.1:.*/v1/introspection/33ce8659-7400-4c68-9535-d10766f07a58/data\\], error message: An error\\\n",
		},
		{
			name: "introspection-status-failed-404-retry-on-wait",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspect wait",
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "introspection-status-failed-extraction",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspecting",
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusBadRequest),

			expectedError: "failed to extract hardware inspection status: Bad request with: \\[GET http://127.0.0.1:.*/v1/introspection/33ce8659-7400-4c68-9535-d10766f07a58\\], error message: An error\\\n",
		},
		{
			name: "introspection-status-failed-404-retry",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: "inspecting",
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "introspection-aborted",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID: nodeUUID,
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospection(nodeUUID, introspection.Introspection{
				Finished: true,
				Error:    "Canceled by operator",
			}),

			expectedResultError: "Canceled by operator",
		},
		{
			name: "inspection-in-progress (not yet finished)",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.Manageable),
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospection(nodeUUID, introspection.Introspection{
				Finished: false,
			}),
			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "inspection-in-progress (but node still in InspectWait)",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectWait),
			}),
			inspector: testserver.NewInspector(t).Ready().
				WithIntrospection(nodeUUID, introspection.Introspection{
					Finished: true,
				}).
				WithIntrospectionData(nodeUUID, introspection.Data{
					Inventory: introspection.InventoryType{
						Hostname: "node-0",
					},
				}),

			expectedDirty:        true,
			expectedRequestAfter: 15,
		},
		{
			name: "inspection-failed",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),

			expectedResultError: "Inspection failed",
		},
		{
			name: "inspection-failed force",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}).NodeUpdate(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.InspectFail),
			}).WithNodeStatesProvisionUpdate(nodeUUID),
			inspector: testserver.NewInspector(t).Ready().WithIntrospectionFailed(nodeUUID, http.StatusNotFound),
			force:     true,

			expectedStarted:      true,
			expectedDirty:        true,
			expectedRequestAfter: 10,
			expectedPublish:      "InspectionStarted Hardware inspection started",
		},
		{
			name: "inspection-complete",
			ironic: testserver.NewIronic(t).Ready().Node(nodes.Node{
				UUID:           nodeUUID,
				ProvisionState: string(nodes.Manageable),
			}),
			inspector: testserver.NewInspector(t).Ready().
				WithIntrospection(nodeUUID, introspection.Introspection{
					Finished: true,
				}).
				WithIntrospectionData(nodeUUID, introspection.Data{
					Inventory: introspection.InventoryType{
						Hostname: "node-0",
					},
				}),

			expectedDirty:       false,
			expectedDetailsHost: "node-0",
			expectedPublish:     "InspectionComplete Hardware inspection completed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.ironic != nil {
				tc.ironic.Start()
				defer tc.ironic.Stop()
			}

			if tc.inspector != nil {
				tc.inspector.Start()
				defer tc.inspector.Stop()
			}

			host := makeHost()
			host.Status.Provisioning.ID = nodeUUID
			publishedMsg := ""
			publisher := func(reason, message string) {
				publishedMsg = reason + " " + message
			}
			auth := clients.AuthConfig{Type: clients.NoAuth}
			prov, err := newProvisionerWithSettings(host, bmc.Credentials{}, publisher,
				tc.ironic.Endpoint(), auth, tc.inspector.Endpoint(), auth,
			)
			if err != nil {
				t.Fatalf("could not create provisioner: %s", err)
			}

			result, started, details, err := prov.InspectHardware(
				provisioner.InspectData{BootMode: metal3v1alpha1.DefaultBootMode},
				tc.force, false)

			assert.Equal(t, tc.expectedStarted, started)
			assert.Equal(t, tc.expectedDirty, result.Dirty)
			assert.Equal(t, time.Second*time.Duration(tc.expectedRequestAfter), result.RequeueAfter)
			assert.Equal(t, tc.expectedResultError, result.ErrorMessage)

			if details != nil {
				assert.Equal(t, tc.expectedDetailsHost, details.Hostname)
			}
			assert.Equal(t, tc.expectedPublish, publishedMsg)
			if tc.expectedError == "" {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Regexp(t, tc.expectedError, err.Error())
			}
		})
	}
}
