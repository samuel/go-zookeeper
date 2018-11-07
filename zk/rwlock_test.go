package zk

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRWLock(t *testing.T) {
	acls := WorldACL(PermAll)
	lockpath := "/lock_test"

	t.Run("should work for single read lock", func(t *testing.T) {
		ts, err := StartTestCluster(1, nil, nil)
		if err != nil {
			panic(err)
		}
		conn, _, err := ts.ConnectAll()
		if err != nil {
			panic(err)
		}
		l1 := NewZKRWLock(conn, lockpath, acls)
		err = l1.RLock()
		assert.NoError(t, err)
		assert.Equal(t, 1, getChildrenCount(conn, lockpath))
		err = l1.RUnlock()
		assert.NoError(t, err)
		assert.Equal(t, 0, getChildrenCount(conn, lockpath))
		afterEach(ts, conn)
	})

	t.Run("should work for write locks", func(t *testing.T) {
		ts, err := StartTestCluster(1, nil, nil)
		if err != nil {
			panic(err)
		}
		conn, _, err := ts.ConnectAll()
		if err != nil {
			panic(err)
		}
		resChan := make(chan int, 2)

		l1 := NewZKRWLock(conn, lockpath, acls)
		err = l1.Lock()
		assert.NoError(t, err)

		go func() {
			l2 := NewZKRWLock(conn, lockpath, acls)
			err := l2.Lock()
			assert.NoError(t, err)

			resChan <- 2
			err = l2.Unlock()
			assert.NoError(t, err)
		}()

		time.Sleep(time.Microsecond * 2000)
		resChan <- 1
		err = l1.Unlock()
		assert.NoError(t, err)

		x := <-resChan
		assert.Equal(t, 1, x)
		x = <-resChan
		assert.Equal(t, 2, x)
		afterEach(ts, conn)
	})

	t.Run("should work that read locks block write locks", func(t *testing.T) {
		ts, err := StartTestCluster(1, nil, nil)
		if err != nil {
			panic(err)
		}
		conn, _, err := ts.ConnectAll()
		if err != nil {
			panic(err)
		}
		resChan := make(chan int, 2)

		l1 := NewZKRWLock(conn, lockpath, acls)
		err = l1.RLock()
		assert.NoError(t, err)

		go func() {
			l2 := NewZKRWLock(conn, lockpath, acls)
			err := l2.Lock()
			assert.NoError(t, err)

			resChan <- 2
			err = l2.Unlock()
			assert.NoError(t, err)
		}()

		time.Sleep(time.Microsecond * 2000)
		resChan <- 1
		err = l1.RUnlock()
		assert.NoError(t, err)

		x := <-resChan
		assert.Equal(t, 1, x)
		x = <-resChan
		assert.Equal(t, 2, x)
		afterEach(ts, conn)
	})

	t.Run("should work for multiple read locks", func(t *testing.T) {
		ts, err := StartTestCluster(1, nil, nil)
		if err != nil {
			panic(err)
		}
		conn, _, err := ts.ConnectAll()
		if err != nil {
			panic(err)
		}
		resChan := make(chan int, 2)

		l1 := NewZKRWLock(conn, lockpath, acls)
		err = l1.RLock()
		assert.NoError(t, err)

		go func() {
			l2 := NewZKRWLock(conn, lockpath, acls)
			err := l2.RLock()
			assert.NoError(t, err)

			resChan <- 2
			err = l2.RUnlock()
			assert.NoError(t, err)
		}()
		time.Sleep(time.Microsecond * 2000)
		resChan <- 1
		err = l1.RUnlock()
		assert.NoError(t, err)

		x := <-resChan
		assert.Equal(t, 2, x)
		x = <-resChan
		assert.Equal(t, 1, x)
		afterEach(ts, conn)
	})

	t.Run("should work that write locks block read locks", func(t *testing.T) {
		ts, err := StartTestCluster(1, nil, nil)
		if err != nil {
			panic(err)
		}
		conn, _, err := ts.ConnectAll()
		if err != nil {
			panic(err)
		}
		resChan := make(chan int, 2)

		l1 := NewZKRWLock(conn, lockpath, acls)
		err = l1.Lock()
		assert.NoError(t, err)

		go func() {
			l2 := NewZKRWLock(conn, lockpath, acls)
			err := l2.RLock()
			assert.NoError(t, err)

			resChan <- 2
			err = l2.RUnlock()
			assert.NoError(t, err)
		}()

		time.Sleep(time.Microsecond * 2000)
		resChan <- 1
		err = l1.Unlock()
		assert.NoError(t, err)

		x := <-resChan
		assert.Equal(t, 1, x)
		x = <-resChan
		assert.Equal(t, 2, x)
		afterEach(ts, conn)
	})

}

func afterEach(ts *TestCluster, conn *Conn) {
	conn.Close()
	ts.Stop()
}

func getChildrenCount(conn *Conn, path string) (c int) {
	children, _, err := conn.Children(path)
	if err != nil {
		panic(err)
	}
	c = len(children)
	return
}
