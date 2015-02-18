NAME=push-mtr
VERSION=$(shell grep Version push-mtr.go | perl -pe 's|.*"([\d\.]+)".*|\1|')

PKG_DIR="$(HOME)/debian/tmp/$(NAME)"
PKG_NAME=$(NAME)-$(VERSION)
PKG=$(PKG_DIR)/$(PKG_NAME).tar.gz
SIG=$(PKG_DIR)/$(PKG_NAME).asc
DEB_TARGET_DIR=$(HOME)/debian/$(NAME)

tarball:
	mkdir -p $(PKG_DIR)
	git archive --output=$(PKG) --prefix=$(PKG_NAME)/ HEAD
	mv $(PKG) $(PKG_DIR)/$(NAME)_$(VERSION).orig.tar.gz

$(SIG): $(PKG)
	gpg --sign --detach-sign --armor $(PKG)

clean:
	rm -rf $(PKG_DIR)
	rm -f $(SIG)

debpkg: tarball 
	mkdir -p $(DEB_TARGET_DIR)
	debuild -S && mv $(PKG_DIR)/* $(DEB_TARGET_DIR)

#test:
#	@$(BATS) test/bootstrap.bat \
#		 test/ip.bat \
#		 test/ssh.bat \
#		 test/ostack_commands.bat \

#.PHONY: test
