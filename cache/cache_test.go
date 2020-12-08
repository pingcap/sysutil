package cache_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"testing"
	"time"

	. "github.com/pingcap/check"
	"github.com/pingcap/sysutil/cache"
)

var _ = SerialSuites(&testCacheSuite{})

func TestT(t *testing.T) {
	TestingT(t)
}

type testCacheSuite struct {
	tmpDir string
}

func (s *testCacheSuite) SetUpSuite(c *C) {
	tmpDir, err := ioutil.TempDir("", "cache")
	c.Assert(err, IsNil)
	s.tmpDir = tmpDir
}

func (s *testCacheSuite) TearDownSuite(c *C) {
	c.Assert(os.RemoveAll(s.tmpDir), IsNil, Commentf("remote tmpDir %v failed", s.tmpDir))
}

func (s *testCacheSuite) prepareFile(c *C, fileName string) (*os.File, os.FileInfo) {
	filePath := path.Join(s.tmpDir, fileName)
	file, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	c.Assert(err, IsNil)
	stat, err := file.Stat()
	c.Assert(err, IsNil)
	return file, stat
}

func (s *testCacheSuite) writeAndReopen(c *C, file *os.File, data string) (*os.File, os.FileInfo) {
	stat, err := file.Stat()
	c.Assert(err, IsNil)
	name := stat.Name()
	_, err = file.WriteString(data)
	c.Assert(err, IsNil)
	err = file.Close()
	c.Assert(err, IsNil)
	return s.prepareFile(c, name)
}

func (s *testCacheSuite) TestLogFileMetaGetStartTime(c *C) {
	fileName := "tidb.log"
	file, stat := s.prepareFile(c, fileName)
	defer file.Close()
	m := cache.NewLogFileMeta(stat)
	c.Assert(m.ModTime, Equals, stat.ModTime())

	// Test GetStartTime meet error
	_, err := m.GetStartTime(stat, nil)
	c.Assert(err.Error(), Equals, "can't get file 'tidb.log' start time")

	_, err = m.GetStartTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("get start time meet error")
	})
	c.Assert(err.Error(), Equals, "get start time meet error")

	// Test GetStartTime
	start := time.Now()
	fileStart, err := m.GetStartTime(stat, func() (time.Time, error) {
		return start, nil
	})
	c.Assert(err, IsNil)
	c.Assert(fileStart.Equal(start), IsTrue)

	// Test GetStartTime from cache
	fileStart, err = m.GetStartTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("should get from cache")
	})
	c.Assert(err, IsNil)
	c.Assert(fileStart.Equal(start), IsTrue)

	// Test GetStartTime from cache
	fileStart, err = m.GetStartTime(stat, nil)
	c.Assert(err, IsNil)
	c.Assert(fileStart.Equal(start), IsTrue)
	// Test GetStartTime with nil stat
	_, err = m.GetStartTime(nil, nil)
	c.Assert(err.Error(), Equals, "file stat can't be nil")

	// Test file has been modified.
	file, stat = s.writeAndReopen(c, file, "a")

	// Test GetStartTime meet invalid error
	_, err = m.GetStartTime(stat, func() (time.Time, error) {
		return time.Now(), cache.InvalidLogFile
	})
	c.Assert(err, Equals, cache.InvalidLogFile)
	c.Assert(m.IsInValid(), IsTrue)

	// Test GetStartTime meet error
	_, err = m.GetStartTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("get start time meet error")
	})
	c.Assert(err.Error(), Equals, "get start time meet error")
	c.Assert(m.IsInValid(), IsTrue)

	newStartTime := time.Now()
	fileStart, err = m.GetStartTime(stat, func() (time.Time, error) {
		return newStartTime, nil
	})
	c.Assert(err, IsNil)
	c.Assert(fileStart.Equal(newStartTime), IsTrue)
	c.Assert(m.IsInValid(), IsFalse)

	// Test GetStartTime from cache after file changed
	fileStart, err = m.GetStartTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("should get from cache")
	})
	c.Assert(err, IsNil)
	c.Assert(fileStart.Equal(newStartTime), IsTrue)
}

func (s *testCacheSuite) TestLogFileMetaGetEndTime(c *C) {
	fileName := "tidb.log"
	file, stat := s.prepareFile(c, fileName)
	defer file.Close()
	m := cache.NewLogFileMeta(stat)
	c.Assert(m.ModTime, Equals, stat.ModTime())

	// Test GetEndTime meet error
	_, err := m.GetEndTime(stat, nil)
	c.Assert(err.Error(), Equals, "can't get file 'tidb.log' end time")

	_, err = m.GetEndTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("get end time meet error")
	})
	c.Assert(err.Error(), Equals, "get end time meet error")

	// Test GetEndTime
	end := time.Now()
	fileEnd, err := m.GetEndTime(stat, func() (time.Time, error) {
		return end, nil
	})
	c.Assert(err, IsNil)
	c.Assert(fileEnd.Equal(end), IsTrue)
	c.Assert(m.IsInValid(), IsFalse)

	// Test GetEndTime from cache
	fileEnd, err = m.GetEndTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("should get from cache")
	})
	c.Assert(err, IsNil)
	c.Assert(fileEnd.Equal(end), IsTrue)

	// Test GetEndTime from cache
	fileEnd, err = m.GetEndTime(stat, nil)
	c.Assert(err, IsNil)
	c.Assert(fileEnd.Equal(end), IsTrue)
	// Test GetEndTime with nil stat
	_, err = m.GetEndTime(nil, nil)
	c.Assert(err.Error(), Equals, "file stat can't be nil")

	// Test file has been modified.
	file, stat = s.writeAndReopen(c, file, "a")

	// Test GetEndTime meet invalid error
	_, err = m.GetEndTime(stat, func() (time.Time, error) {
		return time.Now(), cache.InvalidLogFile
	})
	c.Assert(err, Equals, cache.InvalidLogFile)
	c.Assert(m.IsInValid(), IsTrue)

	// Test GetEndTime meet error
	_, err = m.GetEndTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("get end time meet error")
	})
	c.Assert(err.Error(), Equals, "get end time meet error")

	// Test GetEndTime success
	newEndTime := time.Now()
	fileEnd, err = m.GetEndTime(stat, func() (time.Time, error) {
		return newEndTime, nil
	})
	c.Assert(err, IsNil)
	c.Assert(fileEnd.Equal(newEndTime), IsTrue)
	c.Assert(m.IsInValid(), IsFalse)

	// Test GetEndTime from cache after file changed
	fileEnd, err = m.GetEndTime(stat, func() (time.Time, error) {
		return time.Now(), fmt.Errorf("should get from cache")
	})
	c.Assert(err, IsNil)
	c.Assert(fileEnd.Equal(newEndTime), IsTrue)
	c.Assert(m.IsInValid(), IsFalse)
}

func (s *testCacheSuite) TestLogFileMetaCache(c *C) {
	ca := cache.NewLogFileMetaCache()
	ca.AddFileMataToCache(nil, nil)
	c.Assert(ca.Len(), Equals, 0)
	fileName := "tidb.log"
	file, stat := s.prepareFile(c, fileName)
	defer file.Close()

	m := cache.NewLogFileMeta(stat)
	ca.AddFileMataToCache(stat, m)
	c.Assert(ca.Len(), Equals, 1)
	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.IsInValid(), IsFalse)
	c.Assert(m.CheckFileNotModified(stat), IsTrue)

	m2 := cache.NewLogFileMeta(stat)
	ca.AddFileMataToCache(stat, m2)
	c.Assert(ca.Len(), Equals, 1)
	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.IsInValid(), IsFalse)
	c.Assert(m.CheckFileNotModified(stat), IsTrue)

	ca.AddFileMataToCache(nil, m)
	c.Assert(ca.Len(), Equals, 1)
	ca.AddFileMataToCache(stat, nil)
	c.Assert(ca.Len(), Equals, 1)

	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.IsInValid(), IsFalse)
	c.Assert(m.CheckFileNotModified(stat), IsTrue)

	m = ca.GetFileMata(nil)
	c.Assert(m, IsNil)
}

func (s *testCacheSuite) TestLogFileMetaCacheWithCap(c *C) {
	ca := cache.NewLogFileMetaCacheWithCap(1)
	ca.AddFileMataToCache(nil, nil)
	c.Assert(ca.Len(), Equals, 0)
	fileName := "tidb.log"
	file, stat := s.prepareFile(c, fileName)
	defer file.Close()
	fileName2 := "tidb2.log"
	file2, stat2 := s.prepareFile(c, fileName2)
	defer file2.Close()

	m := cache.NewLogFileMeta(stat)
	ca.AddFileMataToCache(stat, m)
	c.Assert(ca.Len(), Equals, 1)
	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.IsInValid(), IsFalse)
	c.Assert(m.CheckFileNotModified(stat), IsTrue)

	m2 := cache.NewLogFileMeta(stat2)
	ca.AddFileMataToCache(stat2, m2)
	c.Assert(ca.Len(), Equals, 1)
	m = ca.GetFileMata(stat2)
	c.Assert(m, IsNil)

	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.IsInValid(), IsFalse)
	c.Assert(m.CheckFileNotModified(stat), IsTrue)

	fmt.Printf("old mod time: \n%v\n%v ---\n", stat.ModTime(), m.ModTime)
	file, stat = s.writeAndReopen(c, file, "abc")
	fmt.Printf("new mod time: \n%v\n%v ---\n", stat.ModTime(), m.ModTime)
	m = ca.GetFileMata(stat)
	c.Assert(m, NotNil)
	c.Assert(m.CheckFileNotModified(stat), IsFalse)
}
