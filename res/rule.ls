# RULE file aims to be a minimal configuration file format that's easy to read
# due to obvious semantics. There are two parts per line on the RULE file:
# mode and glob. mode is on the left of the space sign and glob is on the
# right. mode is a character that describes whether the host should be accessed
# through a proxy, and the glob is a glob-style string.
#
# Glob patterns:
#   h?llo matches hello, hallo and hxllo
#   h*llo matches hllo and heeeello
#   h[ae]llo matches hello and hallo, but not hillo
#   h[^e]llo matches hallo, hbllo, ... but not hello
#   h[a-b]llo matches hallo and hbllo
#
# This is a normal RULE document:
#   L a.com a.a.com
#   R b.com *.b.com
#   B c.com
#
# L(ocale) means using locale network
# R(emote) means using remote network
# B(anned) means block it
