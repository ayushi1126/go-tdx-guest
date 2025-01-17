// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package linuxabi describes the ABI required for the TDX ioctl commands
package linuxabi

import (
	"fmt"
	"reflect"
	"unsafe"
)

const (
	iocNrbits    = 8
	iocTypebits  = 8
	iocSizebits  = 14
	iocDirbits   = 2
	iocNrshift   = 0
	iocTypeshift = (iocNrshift + iocNrbits)
	iocSizeshift = (iocTypeshift + iocTypebits)
	iocDirshift  = (iocSizeshift + iocSizebits)
	iocWrite     = 1
	iocRead      = 2
	// Linux /dev/tdx-guest ioctl interface
	iocTypeTdxGuestReq = 'T'
	iocTdxWithoutNr    = ((iocWrite | iocRead) << iocDirshift) |
		(iocTypeTdxGuestReq << iocTypeshift) |
		(64 << iocSizeshift)
	// IocTdxGetReport is the ioctl command for getting an attestation report
	IocTdxGetReport = iocTdxWithoutNr | (0x1 << iocNrshift)
	// IocTdxGetQuote is the ioctl command for getting an attestation quote
	IocTdxGetQuote = ((iocRead) << iocDirshift) |
		(iocTypeTdxGuestReq << iocTypeshift) | (0x2 << iocNrshift) |
		(64 << iocSizeshift)
	// TdReportDataSize is a constant for TDX ReportData size
	TdReportDataSize = 64
	// TdReportSize is a constant for TDX Report size
	TdReportSize = 1024
	// HeaderSize is the size of header to serialized quote request
	HeaderSize = 4
	// ReqBufSize is a constant for serialized Tdx quote response
	ReqBufSize = 4 * 4 * 1024
	// TdxUUIDSize is a constant for intel TDQE ID
	TdxUUIDSize = 16
	// GetQuoteReq is a constant for report request
	GetQuoteReq = 0
	// GetQuoteResp is a constant for report response
	GetQuoteResp = 1
	// MsgLibMajorVer is a constant for major version for header
	MsgLibMajorVer = 1
	// MsgLibMinorVer is a constant for minor version for header
	MsgLibMinorVer = 0
	// GetQuotesReqSize is a constant for Quote Request Msg
	GetQuotesReqSize = 24
	// GetQuoteRespSize is a constant for Quote Response Msg
	GetQuoteRespSize = 24
	// TdxQuoteHdrSize is a constant for Quote Header size
	TdxQuoteHdrSize = 24
	// TdQuoteBufSize is a constant denotes buffer size quote request
	TdQuoteBufSize = ReqBufSize - GetQuotesReqSize
	// TdIDQuoteSize is a constant denotes maximum size of array containing Quote
	TdIDQuoteSize = TdQuoteBufSize - HeaderSize - TdxQuoteHdrSize
)

// EsResult is the status code type for Linux's GHCB communication results.
type EsResult int

// constant for TD quote status code.
const (
	GetQuoteSuccess            = 0
	GetQuoteInFlight           = 0xffffffffffffffff
	GetQuoteError              = 0x8000000000000000
	GetQuoteServiceUnavailable = 0x8000000000000001
)
const (
	// TdxAttestSuccess denotes success
	TdxAttestSuccess = iota
	// TdxAttestErrorBusy returns when device driver is busy
	TdxAttestErrorBusy = 0x0009
	// TdxAttestErrorQuoteFailure denotes failure to get the TD Quote
	TdxAttestErrorQuoteFailure = 0x0008
	// TdxAttestErrorNotSupported denotes request feature is not supported
	TdxAttestErrorNotSupported = 0x0007
	// TdxAttestErrorUnexpected denotes Unexpected error
	TdxAttestErrorUnexpected = 0x0001
)

// TdxReportDataABI is Linux's tdx-guest abi for ReportData
type TdxReportDataABI struct {
	// Data is the reportData of 64 bytes
	Data [TdReportDataSize]uint8
}

// ABI returns the object itself.
func (r *TdxReportDataABI) ABI() BinaryConversion { return r }

// Pointer returns a pointer to the object itself.
func (r *TdxReportDataABI) Pointer() unsafe.Pointer { return unsafe.Pointer(r) }

// Finish is a no-op.
func (r *TdxReportDataABI) Finish(BinaryConvertible) error {
	return nil
}

// TdxReportABI is Linux's tdx-guest abi for report response
type TdxReportABI struct {
	// Data is the report response data
	Data [TdReportSize]uint8
}

// ABI returns the object itself.
func (r *TdxReportABI) ABI() BinaryConversion { return r }

// Pointer returns a pointer to the object itself.
func (r *TdxReportABI) Pointer() unsafe.Pointer { return unsafe.Pointer(r) }

// Finish is a no-op.
func (r *TdxReportABI) Finish(BinaryConvertible) error {
	return nil
}

// TdxReportReqABI is Linux's tdx-guest ABI for TDX Report
type TdxReportReqABI struct {
	/* Report data of 64 bytes */
	ReportData unsafe.Pointer
	/* Actual TD Report Data */
	TdReport unsafe.Pointer
}

// TdxReportReq is Linux's tdx-guest ABI for TDX Report. The
// types here enhance runtime safety when using Ioctl as an interface.
type TdxReportReq struct {
	/* Report data of 64 bytes */
	ReportData [TdReportDataSize]uint8
	/* Actual TD Report Data */
	TdReport [TdReportSize]uint8
}

// ABI returns the object itself.
func (r *TdxReportReq) ABI() BinaryConversion {
	return &TdxReportReqABI{
		ReportData: unsafe.Pointer(&r.ReportData[0]),
		TdReport:   unsafe.Pointer(&r.TdReport[0]),
	}
}

// Pointer returns a pointer to the object itself.
func (r *TdxReportReqABI) Pointer() unsafe.Pointer { return unsafe.Pointer(r) }

// Finish writes back the changed value.
func (r *TdxReportReqABI) Finish(b BinaryConvertible) error {
	_, ok := b.(*TdxReportReq)
	if !ok {
		return fmt.Errorf("argument is %v. Expects a *TdxReportReq", reflect.TypeOf(b))
	}
	return nil
}

// MsgHeader is used to add header field to serialized request and response message.
type MsgHeader struct {
	MajorVersion uint16
	MinorVersion uint16
	MsgType      uint32
	Size         uint32 // size of the whole message, include this header, in byte
	ErrorCode    uint32 // used in response only
}

// SerializedGetQuoteReq is used to serialized the request message to get quote.
type SerializedGetQuoteReq struct {
	Header       MsgHeader           // header.type = GET_QUOTE_REQ
	ReportSize   uint32              // cannot be 0
	IDListSize   uint32              // length of id_list, in byte, can be 0
	ReportIDList [TdReportSize]uint8 // report followed by id list - [TODO revisit if attestation key ID is included]
}

// SerializedGetQuoteResp is used to serialized the response message.
type SerializedGetQuoteResp struct {
	Header         MsgHeader            // header.type = GET_QUOTE_RESP
	SelectedIDSize uint32               // can be 0 in case only one id is sent in request
	QuoteSize      uint32               // length of quote_data, in byte
	IDQuote        [TdIDQuoteSize]uint8 // selected id followed by quote -[TODO revisit if attestation key ID is included]
}

// TdxQuoteHdr is Linux's tdx-guest ABI for quote header
type TdxQuoteHdr struct {
	/* Quote version, filled by TD */
	Version uint64
	/* Status code of Quote request, filled by VMM */
	Status uint64
	/* Length of TDREPORT, filled by TD */
	InLen uint32
	/* Length of Quote, filled by VMM */
	OutLen uint32
	/* Actual Quote data or TDREPORT on input */
	Data [TdQuoteBufSize]byte
}

// ABI returns the object itself.
func (r *TdxQuoteHdr) ABI() BinaryConversion { return r }

// Pointer returns a pointer to the object itself.
func (r *TdxQuoteHdr) Pointer() unsafe.Pointer { return unsafe.Pointer(r) }

// Finish is a no-op.
func (r *TdxQuoteHdr) Finish(BinaryConvertible) error {
	return nil
}

// TdxQuoteReqABI is Linux's tdx-guest ABI for quote response
type TdxQuoteReqABI struct {
	Buffer unsafe.Pointer
	Length uint64
}

// TdxQuoteReq is Linux's tdx-guest ABI for TDX Report. The
// types here enhance runtime safety when using Ioctl as an interface.
type TdxQuoteReq struct {
	Buffer BinaryConvertible
	Length uint64
}

// ABI returns the object itself.
func (r *TdxQuoteReq) ABI() BinaryConversion {
	return &TdxQuoteReqABI{
		Buffer: unsafe.Pointer(r.Buffer.ABI().Pointer()),
		Length: r.Length,
	}
}

// Pointer returns a pointer to the object itself.
func (r *TdxQuoteReqABI) Pointer() unsafe.Pointer { return unsafe.Pointer(r) }

// Finish is a no-op.
func (r *TdxQuoteReqABI) Finish(b BinaryConvertible) error {
	_, ok := b.(*TdxQuoteReq)
	if !ok {
		return fmt.Errorf("Finish argument is %v. Expects a *TdxReportReq", reflect.TypeOf(b))
	}
	return nil
}

// BinaryConversion is an interface that abstracts a "stand-in" object that passes through an ABI
// boundary and can finalize changes to the original object.
type BinaryConversion interface {
	Pointer() unsafe.Pointer
	Finish(BinaryConvertible) error
}

// BinaryConvertible is an interface for an object that can produce a partner BinaryConversion
// object to allow its representation to pass the ABI boundary.
type BinaryConvertible interface {
	ABI() BinaryConversion
}
