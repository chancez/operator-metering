// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	v1alpha1 "github.com/operator-framework/operator-metering/pkg/apis/metering/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeHiveTables implements HiveTableInterface
type FakeHiveTables struct {
	Fake *FakeMeteringV1alpha1
	ns   string
}

var hivetablesResource = schema.GroupVersionResource{Group: "metering.openshift.io", Version: "v1alpha1", Resource: "hivetables"}

var hivetablesKind = schema.GroupVersionKind{Group: "metering.openshift.io", Version: "v1alpha1", Kind: "HiveTable"}

// Get takes name of the hiveTable, and returns the corresponding hiveTable object, and an error if there is any.
func (c *FakeHiveTables) Get(name string, options v1.GetOptions) (result *v1alpha1.HiveTable, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(hivetablesResource, c.ns, name), &v1alpha1.HiveTable{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.HiveTable), err
}

// List takes label and field selectors, and returns the list of HiveTables that match those selectors.
func (c *FakeHiveTables) List(opts v1.ListOptions) (result *v1alpha1.HiveTableList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(hivetablesResource, hivetablesKind, c.ns, opts), &v1alpha1.HiveTableList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.HiveTableList{ListMeta: obj.(*v1alpha1.HiveTableList).ListMeta}
	for _, item := range obj.(*v1alpha1.HiveTableList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested hiveTables.
func (c *FakeHiveTables) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(hivetablesResource, c.ns, opts))

}

// Create takes the representation of a hiveTable and creates it.  Returns the server's representation of the hiveTable, and an error, if there is any.
func (c *FakeHiveTables) Create(hiveTable *v1alpha1.HiveTable) (result *v1alpha1.HiveTable, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(hivetablesResource, c.ns, hiveTable), &v1alpha1.HiveTable{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.HiveTable), err
}

// Update takes the representation of a hiveTable and updates it. Returns the server's representation of the hiveTable, and an error, if there is any.
func (c *FakeHiveTables) Update(hiveTable *v1alpha1.HiveTable) (result *v1alpha1.HiveTable, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(hivetablesResource, c.ns, hiveTable), &v1alpha1.HiveTable{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.HiveTable), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeHiveTables) UpdateStatus(hiveTable *v1alpha1.HiveTable) (*v1alpha1.HiveTable, error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateSubresourceAction(hivetablesResource, "status", c.ns, hiveTable), &v1alpha1.HiveTable{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.HiveTable), err
}

// Delete takes name of the hiveTable and deletes it. Returns an error if one occurs.
func (c *FakeHiveTables) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(hivetablesResource, c.ns, name), &v1alpha1.HiveTable{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeHiveTables) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(hivetablesResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &v1alpha1.HiveTableList{})
	return err
}

// Patch applies the patch and returns the patched hiveTable.
func (c *FakeHiveTables) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.HiveTable, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(hivetablesResource, c.ns, name, pt, data, subresources...), &v1alpha1.HiveTable{})

	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.HiveTable), err
}
