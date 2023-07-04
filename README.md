# TDX Guest


This project offers libraries for a simple wrapper around the `/dev/tdx-guest`
device in Linux, as well as a library for attestation verification of
fundamental components of an attestation quote.


This project is split into two complementary roles. The first role is producing
an attestation quote, and the second is checking an attestation quote. The
`client` library produces quote, the `verify` library verifies quote's
signatures and certificates.


## `client`


This library should be used within the confidential workload to collect an
attestation quote along with requisite certificates.


Your main interactions with it will be to open the device, get an attestation
quote with your provided 64 bytes of user data (typically a nonce), and then
close the device. For convenience, the attestation with its associated
certificates can be collected in a wire-transmittable protocol buffer format.


### `func OpenDevice() (*LinuxDevice, error)`


This function creates a file descriptor to the `/dev/tdx-guest` device and
returns an object that has methods encapsulating commands to the device. When
done, remember to `Close()` the device.


### `func GetQuote(d Device, reportData [64]byte) (*pb.QuoteV4, error)`


This function takes an object implementing the `Device` interface (e.g., a
`LinuxDevice`) and returns the protocol buffer representation of the attestation
quote.


You can use `GetRawQuote` to get the TDX Quote in byte array format.


### `func (d Device) Close() error`


Closes the device.


## License


go-tdx-guest is released under the Apache 2.0 license.


```
Copyright 2023 Google LLC


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at


http://www.apache.org/licenses/LICENSE-2.0


Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```


## Links


* [Intel TDX specification](https://cdrdv2.intel.com/v1/dl/getContent/733568)
* [Intel PCK Certificate specification](https://api.trustedservices.intel.com/documents/Intel_SGX_PCK_Certificate_CRL_Spec-1.5.pdf)
* [Intel PCS API specification](https://api.portal.trustedservices.intel.com/provisioning-certification)


## Disclaimers


This is not an officially supported Google product.