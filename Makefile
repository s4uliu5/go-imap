include $(GOROOT)/src/Make.inc

GCIMPORTS=-Iimap/_obj
LDIMPORTS=-Limap/_obj

TARG=imapsync
GOFILES=\
	main.go\
	mbox.go\

include $(GOROOT)/src/Make.cmd
