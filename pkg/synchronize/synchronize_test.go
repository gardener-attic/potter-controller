package synchronize

import (
	"context"
	"testing"
	"time"

	"github.com/arschles/assert"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	hubtesting "github.wdf.sap.corp/kubernetes/hub-controller/pkg/testing"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"
)

func TestMutexMap(t *testing.T) {
	keyA := types.NamespacedName{Name: "A"}
	keyB := types.NamespacedName{Name: "B"}

	mutexMap := newMutexMap()

	mutex1 := mutexMap.getMutex(&keyA)

	assert.NotNil(t, mutex1, "mutex 1")
	assert.Equal(t, len(mutexMap.syncMap), 1, "length")
	assert.Equal(t, mutexMap.syncMap[keyA].counter, 1, "counter")

	mutex2 := mutexMap.getMutex(&keyA)
	assert.Equal(t, mutex2, mutex1, "mutex 2")
	assert.Equal(t, len(mutexMap.syncMap), 1, "length")
	assert.Equal(t, mutexMap.syncMap[keyA].counter, 2, "counter")

	mutex3 := mutexMap.getMutex(&keyB)
	assert.NotNil(t, mutex3, "mutex 3")
	assert.Equal(t, len(mutexMap.syncMap), 2, "length")
	assert.Equal(t, mutexMap.syncMap[keyA].counter, 2, "counter for key A")
	assert.Equal(t, mutexMap.syncMap[keyB].counter, 1, "counter for key B")

	mutexMap.releaseMutex(&keyA)
	assert.Equal(t, len(mutexMap.syncMap), 2, "length")
	assert.Equal(t, mutexMap.syncMap[keyA].counter, 1, "counter for key A")
	assert.Equal(t, mutexMap.syncMap[keyB].counter, 1, "counter for key B")

	mutexMap.releaseMutex(&keyA)
	assert.Equal(t, len(mutexMap.syncMap), 1, "length")
	assert.Equal(t, mutexMap.syncMap[keyB].counter, 1, "counter for key B")

	mutexMap.releaseMutex(&keyB)
	assert.Equal(t, len(mutexMap.syncMap), 0, "length")
}

func TestBlockObject_Block_Extend(t *testing.T) {
	key := types.NamespacedName{Namespace: "testNamespace", Name: "testName"}
	ctx := newTestContext()
	cli := hubtesting.NewUnitTestClientDi()
	uniqueID := "A"
	clock := testClock{}
	blockObject := newTestBlockObject(uniqueID, &clock)

	// Create block
	t1 := newTestTime(12, 0)
	t2 := newTestTime(13, 20)
	clock.SetTime(t1)
	ok, _, err := blockObject.Block(ctx, &key, cli, t2.Sub(t1), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	block := hubv1.ClusterBomSync{}
	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t2, "until")

	// Extend block
	t3 := newTestTime(13, 0)
	t4 := newTestTime(14, 40)
	clock.SetTime(t3)
	ok, _, err = blockObject.Block(ctx, &key, cli, t4.Sub(t3), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t4, "until")

	// No extension, because new expiration date t6 is before current expiration date t4
	t5 := newTestTime(14, 0)
	t6 := newTestTime(14, 20)
	clock.SetTime(t5)
	ok, _, err = blockObject.Block(ctx, &key, cli, t6.Sub(t5), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t4, "until")

	// Extend own expired block
	t7 := newTestTime(15, 0)
	t8 := newTestTime(16, 0)
	clock.SetTime(t7)
	ok, _, err = blockObject.Block(ctx, &key, cli, t8.Sub(t7), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t8, "until")
}

func TestBlockObject_Reblock(t *testing.T) {
	key := types.NamespacedName{Namespace: "testNamespace", Name: "testName"}
	ctx := newTestContext()
	cli := hubtesting.NewUnitTestClientDi()
	uniqueID := "A"
	clock := testClock{}
	blockObject := newTestBlockObject(uniqueID, &clock)

	// Try to reblock a block that does not exist
	t1 := newTestTime(12, 0)
	t2 := newTestTime(13, 20)
	clock.SetTime(t1)
	ok, err := blockObject.Reblock(ctx, &key, cli, t2.Sub(t1), false)

	assert.Equal(t, ok, false, "block ok")
	assert.NotNil(t, err, "block error")

	block := hubv1.ClusterBomSync{}
	err = cli.Get(ctx, key, &block)

	assert.NotNil(t, err, "error")

	// Create block
	clock.SetTime(t1)
	ok, _, err = blockObject.Block(ctx, &key, cli, t2.Sub(t1), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	block = hubv1.ClusterBomSync{}
	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t2, "until")

	// Reblock own block that is not expired
	t3 := newTestTime(13, 0)
	t4 := newTestTime(14, 40)
	clock.SetTime(t3)
	ok, err = blockObject.Reblock(ctx, &key, cli, t4.Sub(t3), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t4, "until")

	// Reblock of own expired block fails
	t5 := newTestTime(15, 0)
	t6 := newTestTime(16, 0)
	clock.SetTime(t5)
	ok, err = blockObject.Reblock(ctx, &key, cli, t6.Sub(t5), false)

	assert.Equal(t, ok, false, "block ok")
	assert.NotNil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t4, "until")
}

func TestBlockObject_Block_Takeover(t *testing.T) {
	key := types.NamespacedName{Namespace: "testNamespace", Name: "testName"}
	ctx := newTestContext()
	cli := hubtesting.NewUnitTestClientDi()
	uniqueID := "A"
	clock := testClock{}
	blockObject := newTestBlockObject(uniqueID, &clock)

	// Create block
	t1 := newTestTime(12, 0)
	t2 := newTestTime(13, 20)
	clock.SetTime(t1)
	ok, _, err := blockObject.Block(ctx, &key, cli, t2.Sub(t1), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	block := hubv1.ClusterBomSync{}
	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t2, "until")

	// Failing takeover, because block is not yet expired
	uniqueID2 := "B"
	blockObject2 := newTestBlockObject(uniqueID2, &clock)
	t3 := newTestTime(13, 0)
	t4 := newTestTime(14, 40)
	clock.SetTime(t3)

	ok, retryDuration, err := blockObject2.Block(ctx, &key, cli, t4.Sub(t3), false)

	assert.Equal(t, ok, false, "block ok")
	assert.Equal(t, retryDuration, t2.Add(time.Minute).Add(time.Second*5).Sub(t3), "blocked until")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t2, "until")

	// Successful takeover of an expired block
	t5 := newTestTime(15, 0)
	t6 := newTestTime(16, 0)
	clock.SetTime(t5)
	ok, _, err = blockObject2.Block(ctx, &key, cli, t6.Sub(t5), false)

	assert.Equal(t, ok, true, "block ok")
	assert.Nil(t, err, "block error")

	err = cli.Get(ctx, key, &block)

	assert.Nil(t, err, "error")
	assert.Equal(t, len(cli.ClusterBomSyncs), 1, "number of blocks")
	assert.Equal(t, block.Spec.ID, uniqueID2, "id")
	assert.Equal(t, block.Spec.Timestamp.Time, t1, "timestamp")
	assert.Equal(t, block.Spec.Until.Time, t6, "until")
}

func newTestTime(hour, minute int) time.Time {
	return time.Date(2000, 5, 15, hour, minute, 0, 0, time.UTC)
}

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time {
	return c.now
}

func (c *testClock) SetTime(t time.Time) {
	c.now = t
}

func newTestBlockObject(uniqueID string, clock util.Clock) *BlockObject {
	return &BlockObject{
		mutexHelper:  newMutexMap(),
		uniqueID:     uniqueID,
		excludedBoms: nil,
		clock:        clock,
	}
}

func newTestContext() context.Context {
	log := ctrl.Log.WithName("controllers").WithName("UnitTest")
	ctx := context.Background()
	ctx = context.WithValue(ctx, util.LoggerKey{}, log)
	return ctx
}
