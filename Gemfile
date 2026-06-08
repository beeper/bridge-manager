# frozen_string_literal: true

source 'https://rubygems.org'

gem 'fastlane', '~> 2.235'
gem 'fastlane-plugin-wpmreleasetoolkit', '~> 14.6'
# fastlane <= 2.235.0 crashes at startup on Ruby 3.3+ with "multi_json is not
# part of the bundle": Google stopped pulling multi_json transitively
# (googleapis/google-api-ruby-client#26611) and fastlane eagerly loads its
# Google Play actions, which require it through representable. fastlane re-added
# it as a direct dependency for 2.236.0 (fastlane/fastlane#30062) — drop this
# line once the lock is on fastlane >= 2.236.0.
gem 'multi_json'
