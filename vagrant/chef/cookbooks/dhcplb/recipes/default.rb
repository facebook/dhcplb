# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

node.default['go']['version'] = '1.13'
node.default['go']['packages'] = ['github.com/facebook/dhcplb']

include_recipe 'golang'
include_recipe 'golang::packages'

directory '/home/vagrant/go' do
  owner 'vagrant'
  group 'vagrant'
  recursive true
end

cookbook_file '/home/vagrant/dhcplb.config.json' do
  source 'dhcplb.config.json'
  notifies :restart, 'poise_service[dhcplb]'
end

template '/home/vagrant/dhcp-servers-v4.cfg' do
  source 'dhcp-servers-v4.cfg.erb'
  # dhcplb will auto load files that change. no need to notify.
end

# Configure service via https://github.com/poise/poise-service
poise_service 'dhcplb' do
  command '/opt/go/bin/dhcplb -version 4 -config /home/vagrant/dhcplb.config.json'
end
