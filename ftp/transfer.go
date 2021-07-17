package ftp

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

func (c *Client) Retrieve(path string, dest io.Writer) error {
	size, err := c.size(path)
	if err != nil {
		return err
	}
	canResume := c.canResume()
	var bytesSoFar int64
	for {
		n, err := c.transferFromOffset(path, dest, nil, bytesSoFar)
		bytesSoFar += n
		if err == nil {
			break
		} else if n == 0 {
			return err
		} else if !canResume {
			return ftpError{
				err:       fmt.Errorf("%s (can't resume)", err),
				temporary: true,
			}
		}
	}
	if size != -1 && bytesSoFar != size {
		return ftpError{
			err:       fmt.Errorf("expected %d bytes, got %d", size, bytesSoFar),
			temporary: true,
		}
	}
	return nil
}

func (c *Client) Store(path string, src io.Reader) error {
	canResume := len(c.hosts) == 1 && c.canResume()
	seeker, ok := src.(io.Seeker)
	if !ok {
		canResume = false
	}
	var (
		bytesSoFar int64
		err        error
		n          int64
	)
	for {
		if bytesSoFar > 0 {
			size, sizeErr := c.size(path)
			if sizeErr != nil {
				return ftpError{
					err:       sizeErr,
					temporary: true,
				}
			}
			if size == -1 {
				return ftpError{
					err:       fmt.Errorf("%s (resume failed)", err),
					temporary: true,
				}
			}
			_, seekErr := seeker.Seek(size, os.SEEK_SET)
			if seekErr != nil {
				c.debug("failed seeking to %d while resuming upload to %s: %s",
					size,
					path,
					err,
				)
				return ftpError{
					err:       fmt.Errorf("%s (resume failed)", err),
					temporary: true,
				}
			}
			bytesSoFar = size
		}
		n, err = c.transferFromOffset(path, nil, src, bytesSoFar)
		bytesSoFar += n
		if err == nil {
			break
		} else if n == 0 {
			return ftpError{
				err:       err,
				temporary: true,
			}
		} else if !canResume {
			return ftpError{
				err:       fmt.Errorf("%s (can't resume)", err),
				temporary: true,
			}
		}
	}
	size, err := c.size(path)
	if err != nil {
		return err
	}
	if size != -1 && size != bytesSoFar {
		return ftpError{
			err:       fmt.Errorf("sent %d bytes, but size is %d", bytesSoFar, size),
			temporary: true,
		}
	}
	return nil
}

func (c *Client) transferFromOffset(path string, dest io.Writer, src io.Reader, offset int64) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return 0, err
	}
	defer c.returnConn(pconn)
	if err = pconn.setType("I"); err != nil {
		return 0, err
	}
	if offset > 0 {
		err := pconn.sendCommandExpected(replyFileActionPending, "REST %d", offset)
		if err != nil {
			return 0, err
		}
	}
	connGetter, err := pconn.prepareDataConn()
	if err != nil {
		pconn.debug("error preparing data connection: %s", err)
		return 0, err
	}
	var cmd string
	if dest == nil && src != nil {
		cmd = "STOR"
	} else if dest != nil && src == nil {
		cmd = "RETR"
	} else {
		panic("this shouldn't happen")
	}
	err = pconn.sendCommandExpected(replyGroupPreliminaryReply, "%s %s", cmd, path)
	if err != nil {
		return 0, err
	}
	dc, err := connGetter()
	if err != nil {
		pconn.debug("error getting data connection: %s", err)
		return 0, err
	}
	defer dc.Close()
	if dest == nil {
		dest = dc
	} else {
		src = dc
	}
	n, err := io.Copy(dest, src)
	if err != nil {
		pconn.broken = true
		return n, err
	}
	err = dc.Close()
	if err != nil {
		pconn.debug("error closing data connection: %s", err)
	}
	code, msg, err := pconn.readResponse()
	if err != nil {
		pconn.debug("error reading response after %s: %s", cmd, err)
		return n, err
	}
	if !positiveCompletionReply(code) {
		pconn.debug("unexpected response after %s: %d (%s)", cmd, code, msg)
		return n, ftpError{code: code, msg: msg}
	}
	return n, nil
}

func (c *Client) size(path string) (int64, error) {
	pconn, err := c.getIdleConn()
	if err != nil {
		return -1, err
	}
	defer c.returnConn(pconn)
	if !pconn.hasFeature("SIZE") {
		pconn.debug("server doesn't support SIZE")
		return -1, nil
	}
	if err = pconn.setType("I"); err != nil {
		return 0, err
	}
	code, msg, err := pconn.sendCommand("SIZE %s", path)
	if err != nil {
		return -1, err
	}
	if code != replyFileStatus {
		pconn.debug("unexpected SIZE response: %d (%s)", code, msg)
		return -1, nil
	}
	size, err := strconv.ParseInt(msg, 10, 64)
	if err != nil {
		pconn.debug(`failed parsing SIZE response "%s": %s`, msg, err)
		return -1, nil
	}
	return size, nil
}

func (c *Client) canResume() bool {
	pconn, err := c.getIdleConn()
	if err != nil {
		return false
	}
	defer c.returnConn(pconn)
	return pconn.hasFeatureWithArg("REST", "STREAM")
}
