package logging

const shaLogPrefixLength = 7 // Git's conventional abbreviated SHA length.

// ShortSHA returns a 7-byte prefix of a SHA for compact log output, or the whole
// string if it is shorter.
func ShortSHA(sha string) string {
	if len(sha) > shaLogPrefixLength {
		return sha[:shaLogPrefixLength]
	}
	return sha
}
