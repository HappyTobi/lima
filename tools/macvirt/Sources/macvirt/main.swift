/**
 Licensed to the Apache Software Foundation (ASF) under one
 or more contributor license agreements.  See the NOTICE file
 distributed with this work for additional information
 regarding copyright ownership.  The ASF licenses this file
 to you under the Apache License, Version 2.0 (the
 "License"); you may not use this file except in compliance
 with the License.  You may obtain a copy of the License at
 
 http://www.apache.org/licenses/LICENSE-2.0
 
 Unless required by applicable law or agreed to in writing,
 software distributed under the License is distributed on an
 "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 KIND, either express or implied.  See the License for the
 specific language governing permissions and limitations
 under the License.
 */

//https://linux.die.net/man/3/cfmakeraw

import Darwin
import ArgumentParser
import Virtualization

private var textLog : Logger? = nil

let vzVirtMachineConfig = VZVirtualMachineConfiguration()

struct Logger: TextOutputStream {
    func write(_ string: String) {
        let paths = FileManager.default.urls(for: .documentDirectory, in: .allDomainsMask)
        let documentDirectoryPath = paths.first!
        let log = documentDirectoryPath.appendingPathComponent("log.txt")
        
        do {
            let handle = try FileHandle(forWritingTo: log)
            handle.seekToEndOfFile()
            handle.write(string.data(using: .utf8)!)
            handle.closeFile()
        } catch {
            print(error.localizedDescription)
            do {
                try string.data(using: .utf8)?.write(to: log)
            } catch {
                print(error.localizedDescription)
            }
        }
        
    }
}

struct Macvirt: ParsableCommand {
    public static let
        configuration = CommandConfiguration(
            abstract: "Macvirt is a small cli wrapper to use the osx internal virtualization feature"
        )
    
    @Option(name: .long, help: "Memory in Megabyte")
    private var memory: UInt64 = 256
    
    @Option(name: .long, help: "CPU cores")
    private var cpuCount: Int = 1
    
    @Option(name: .long, help: "Kernel path")
    private var kernelPath: String
    
    @Option(name: .long, help: "Initrd path")
    private var initrdPath: String
    
    @Option(name: .long, help: "Disk path")
    private var diskPath: String

    @Option(name: .long, help: "Cloud-init data path")
    private var cloudInitDataPath: String
    
    @Option(name: .long, help: "Kernel cmdline arguments")
    private var cmdLineArg: String
    
    @Option(help: "Escape Sequence, when using a tty")
    var escapeSequence: String = "q"
    
    
    func run() throws {
        //configure the bootloader
        let kernelUrl = URL(fileURLWithPath: kernelPath)
        let vzBootLoader = VZLinuxBootLoader(kernelURL: kernelUrl)
        vzBootLoader.initialRamdiskURL = URL(fileURLWithPath: initrdPath)
        vzBootLoader.commandLine = cmdLineArg
        
        try vzVirtMachineConfig.storageDevices.append(attachDisk(diskPath: diskPath,readOnly: false))
        try vzVirtMachineConfig.storageDevices.append(attachDisk(diskPath: cloudInitDataPath,readOnly: true))
        
        let vzTradMemoryBallonDevice = VZVirtioTraditionalMemoryBalloonDeviceConfiguration()
        
        
        //tty setup
        let fhIn = Pipe()
        let fhOut = Pipe()
        
        let vzVirtConsoleDeviceSerialPortConfig = VZVirtioConsoleDeviceSerialPortConfiguration()
        let vzFileHandlerSerialPortAttachment = VZFileHandleSerialPortAttachment(fileHandleForReading: fhIn.fileHandleForReading, fileHandleForWriting: fhOut.fileHandleForWriting)
        
        vzVirtConsoleDeviceSerialPortConfig.attachment = vzFileHandlerSerialPortAttachment
        
        //Network setup
        let vzVirtNetworkDeviceConfig = VZVirtioNetworkDeviceConfiguration()
        vzVirtNetworkDeviceConfig.attachment = VZNATNetworkDeviceAttachment()
        let vzMacAdress = VZMACAddress.randomLocallyAdministered()
        vzVirtNetworkDeviceConfig.macAddress = vzMacAdress
        
        print("Used MAC Adress to connect to the vm it \(vzMacAdress)")
        
        
        vzVirtMachineConfig.bootLoader = vzBootLoader
        vzVirtMachineConfig.cpuCount = cpuCount
        vzVirtMachineConfig.memorySize = memory * 1024 * 1024
        vzVirtMachineConfig.serialPorts = [ vzVirtConsoleDeviceSerialPortConfig ]
        vzVirtMachineConfig.memoryBalloonDevices = [ vzTradMemoryBallonDevice ]
        vzVirtMachineConfig.entropyDevices = [ VZVirtioEntropyDeviceConfiguration() ]
        vzVirtMachineConfig.networkDevices.append(vzVirtNetworkDeviceConfig)
        
        
        try vzVirtMachineConfig.validate()
        

        FileHandle.standardInput.waitForDataInBackgroundAndNotify()
        NotificationCenter.default.addObserver(forName: NSNotification.Name.NSFileHandleDataAvailable, object: FileHandle.standardInput, queue: nil)
        {_ in
            let aData = FileHandle.standardInput.availableData
            fhIn.fileHandleForWriting.write(aData)
            if aData.count > 0 {
                FileHandle.standardInput.waitForDataInBackgroundAndNotify()
            }
        }
        
        fhOut.fileHandleForReading.waitForDataInBackgroundAndNotify()
        NotificationCenter.default.addObserver(forName: NSNotification.Name.NSFileHandleDataAvailable, object: fhOut.fileHandleForReading, queue: nil)
        {_ in
            let aData = fhOut.fileHandleForReading.availableData
            
            FileHandle.standardOutput.write(aData)
            if aData.count > 0 {
                fhOut.fileHandleForReading.waitForDataInBackgroundAndNotify()
            }
        }
        
         //start the vm
         let vm = VZVirtualMachine(configuration: vzVirtMachineConfig)
         vm.start(completionHandler: {(result: Result<Void, Error>) -> Void in
         switch result {
         case .success:
            return
         case .failure(let error):
            FileHandle.standardError.write(error.localizedDescription.data(using: .utf8)!)
            FileHandle.standardError.write("\n".data(using: .utf8)!)
         return //exit(001)
         }
         })
         
         RunLoop.main.run()
    }
}

func attachDisk(diskPath: String, readOnly: Bool) throws -> VZVirtioBlockDeviceConfiguration {
    let diskUrl = URL(fileURLWithPath: diskPath)
    let vzDiskImageStorageDevice: VZDiskImageStorageDeviceAttachment
    do {
        vzDiskImageStorageDevice = try VZDiskImageStorageDeviceAttachment(url: diskUrl, readOnly: readOnly)
        return VZVirtioBlockDeviceConfiguration(attachment: vzDiskImageStorageDevice)
    } catch {
        throw error
    }
}

Macvirt.main()
