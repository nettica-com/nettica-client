Name:           nettica
Version:        2.6
Release:        4%{?dist}
Summary:        Nettica Client for RPM based linux systems

License:        MIT
URL:            https://nettica.com
BuildRoot:      ~/rpmbuild/

%description
Nettica Agent for RPM based linux distributions

%prep
################################################################################
# Create the build tree and copy the files from the development directories    #
# into the build tree.                                                         #
################################################################################
echo "BUILDROOT = $RPM_BUILD_ROOT"
mkdir -p $RPM_BUILD_ROOT/usr/bin/
mkdir -p $RPM_BUILD_ROOT/etc/nettica/
mkdir -p $RPM_BUILD_ROOT/lib/systemd/system/

cp ~/go/src/nettica-client/nettica-client $RPM_BUILD_ROOT/usr/bin
cp ~/go/src/nettica-client/rpmbuild/BUILD/lib/systemd/system/nettica.service $RPM_BUILD_ROOT/lib/systemd/system/nettica.service
exit


%files
%attr(0744, root, root) /usr/bin/nettica-client
%attr(0644, root, root) /lib/systemd/system/nettica.service
%doc


%clean
rm -rf $RPM_BUILD_ROOT/usr/
rm -rf $RPM_BUILD_ROOT/lib/
rm -rf $RPM_BUILD_ROOT/etc/

%post
/usr/bin/systemctl enable nettica.service > /dev/null 2>&1
exit 0
%preun
/usr/bin/systemctl stop nettica.service
exit 0
%posttrans
/usr/bin/systemctl restart nettica.service

%changelog
* Tue Nov 16 2021 by ALan Graham
Support client managed private keys
* Tue Nov 09 2021 by Alan Graham
Improve wireguard conf file generation
* Thu Aug 26 2021 by Alan Graham
Initial Release
