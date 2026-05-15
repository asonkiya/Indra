Pod::Spec.new do |s|
  s.name         = "Indra"
  s.version      = "0.1.0"
  s.summary      = "Indra P2P messaging core (gomobile)"
  s.homepage     = "https://github.com/aryaman/indra"
  s.license      = { :type => "MIT" }
  s.author       = "Aryaman Sonkiya"
  s.source       = { :path => "." }

  s.ios.deployment_target = "15.1"

  s.vendored_frameworks = "build/Indra.xcframework"
  s.static_framework    = true

  # Go runtime requires libresolv for DNS and CoreFoundation/Security for networking
  s.libraries  = "resolv"
  s.frameworks = "CoreFoundation", "Security", "UIKit"
end
