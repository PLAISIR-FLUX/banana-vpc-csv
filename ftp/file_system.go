package ftp

func (c *Client) Delete(path string) error {
	pconn, err := c.getIdleConn()
	if err != nil {
		return err
	}
	defer c.returnConn(pconn)
	return pconn.sendCommandExpected(replyFileActionOkay, "DELE %s", path)
}
