NAME=push-mtr
VERSION=$(shell grep Version push-mtr.go | perl -pe 's|.*"([\d\.]+)".*|\1|')

BUILD_DIR=$(HOME)/debian/tmp/$(NAME)
PKG_NAME=$(NAME)-$(VERSION)
PKG=$(BUILD_DIR)/$(PKG_NAME).tar.gz
DEB_TARGET_DIR=$(HOME)/debian/$(NAME)

all: build

tarball:
	mkdir -p $(BUILD_DIR)
	git archive --output=$(PKG) --prefix=$(PKG_NAME)/ HEAD
	mv $(PKG) $(BUILD_DIR)/$(NAME)_$(VERSION).orig.tar.gz

clean:
	rm -rf $(BUILD_DIR)
	rm -f $(SIG)

debpkg: tarball 
	# Prevents overwriting a good tarball
	! test -f $(DEB_TARGET_DIR)/$(NAME)_$(VERSION).orig.tar.gz
	cd $(BUILD_DIR) && \
		tar xzf $(NAME)_$(VERSION).orig.tar.gz && \
	  cd $(NAME)-$(VERSION) && \
		debuild -S && rm -rf $(BUILD_DIR)/$(NAME)-$(VERSION) && \
		mv $(BUILD_DIR)/* $(DEB_TARGET_DIR)
	rm -rf $(BUILD_DIR)

build:
	GOPATH=$(PWD)/vendor go build

update-deps:
	rm -rf vendor/src
	GOPATH=$(PWD)/vendor go get
	rm -rf vendor/pkg
	find vendor/src -type d -name '.git' -o -name '.hg' | xargs rm -rf
	find vendor/src -type f -name '.gitignore' -o -name '.hgignore' | xargs rm
