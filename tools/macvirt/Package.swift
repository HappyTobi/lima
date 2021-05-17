// swift-tools-version:5.3
// The swift-tools-version declares the minimum version of Swift required to build this package.

import PackageDescription

let package = Package(
    name: "macvirt",
    platforms: [
        .macOS(.v11)
    ],
    products: [
        .executable(name: "macvirt", targets: ["macvirt"])
    ],
    dependencies: [
        .package(url: "https://github.com/apple/swift-argument-parser", from: "0.4.0")
    ],
    targets: [
        .target(
            name: "macvirt",
            dependencies: [
                .product(name: "ArgumentParser", package: "swift-argument-parser")
            ]),
        /*.testTarget(
            name: "macvirtTests",
            dependencies: ["macvirt"]),*/
    ]
)
