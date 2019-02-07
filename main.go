package gopfs0

import (
	"encoding/binary"
	"errors"
	"log"
	"os"
	"path"
	"strings"
)

var err error

const (
	chunkSize = 0x800 // 2048
	magic     = "PFS0"
)

// NewPFS0 creates a new PFS0 object from given filepath
func NewPFS0(filepath string) *PFS0 {
	return &PFS0{Filepath: filepath, Basename: strings.Split(path.Base(filepath), ".")[0]}
}

// PFS0 struct to represent PFS0 filesystem of NSP
type PFS0 struct {
	Filepath  string
	Basename  string
	Size      uint64
	HeaderLen uint16
	Files     []pfs0File
}

// ReadMetadata reads metadata from NSP header and populates PFS0 fields
func (p *PFS0) ReadMetadata() error {
	fileHandle, err := os.Open(p.Filepath)
	if err != nil {
		log.Println(err)
		return err
	}
	defer fileHandle.Close()

	fileHandle.Seek(0, 0)

	fi, err := fileHandle.Stat()
	if err != nil {
		log.Print(err)
		return err
	}
	p.Size = uint64(fi.Size())

	nspHeader := make([]byte, 0x20)
	_, err = fileHandle.Read(nspHeader)
	if string(nspHeader[:0x4]) != magic {
		return errors.New("Invalid NSP header. Expected 'PFS0', got '" + string(nspHeader[:0x4]) + "'")
	}

	fileCount := binary.LittleEndian.Uint16(nspHeader[0x4:0x8])
	p.HeaderLen = 0x10 + (0x18 * fileCount)

	stringsLen := binary.LittleEndian.Uint16(nspHeader[0x8:0xC])
	fileNamesBuffer := make([]byte, stringsLen)
	fileHandle.Seek(int64(p.HeaderLen), 0)
	fileHandle.Read(fileNamesBuffer)

	// Individual file metadata
	p.Files = make([]pfs0File, fileCount)
	for i := uint16(0); i < fileCount; i++ {
		fileHandle.Seek(int64(0x10+(0x18*i)), 0)

		fileMetaData := make([]byte, 0x18)
		_, err = fileHandle.Read(fileMetaData)
		if err != nil {
			log.Print(err)
			return err
		}

		fileOffset := binary.LittleEndian.Uint64(fileMetaData[0:8])
		fileSize := binary.LittleEndian.Uint64(fileMetaData[8:16])
		var nameBytes []byte
		for _, b := range fileNamesBuffer[binary.LittleEndian.Uint16(fileMetaData[16:20]):] {
			if b == 0x0 {
				break
			} else {
				nameBytes = append(nameBytes, b)
			}
		}

		p.Files[i] = pfs0File{fileOffset, fileSize, string(nameBytes)}
	}
	return nil
}

// ReadTik reads ticket file in PFS0 into byte array
func (p *PFS0) ReadTik() ([]byte, error) {
	var tikInd int
	for i, f := range p.Files {
		if f.Name[len(f.Name)-3:] == "tik" {
			tikInd = i
			break
		}
	}

	fileHandle, err := os.Open(p.Filepath)
	if err != nil {
		log.Println(err)
		return nil, err
	}
	defer fileHandle.Close()

	fileHandle.Seek(0, 0)
	tikOffset := uint64(p.HeaderLen) + uint64(p.Files[tikInd].StartOffset)
	fileHandle.Seek(int64(tikOffset), 0)
	ticket := make([]byte, p.Files[tikInd].Size)
	_, err = fileHandle.Read(ticket)
	if err != nil {
		log.Print(err)
		return nil, err
	}
	return ticket, nil
}

// NcaReader returns a channel that reads 0x800byte chunks from the file with
//	the given index in the PFS0 file system
func (p *PFS0) NcaReader(ind uint16) (<-chan chunk, error) {
	fileHandle, err := os.Open(p.Filepath)
	if err != nil {
		log.Println(err)
		return nil, err
	}

	c := make(chan chunk)

	file := p.Files[ind]

	currentOffset := uint64(p.HeaderLen) + uint64(file.StartOffset)
	remaining := file.Size
	fileHandle.Seek(int64(currentOffset), 0)

	go func() {
		defer close(c)
		defer fileHandle.Close()
		for remaining > 0 {
			chnk := chunk{}

			chnk.Content = make([]byte, chunkSize)
			chnk.Size = chunkSize
			if remaining < chunkSize {
				chnk.Content = make([]byte, remaining)
				chnk.Size = remaining
			}

			r, err := fileHandle.Read(chnk.Content)
			chnk.Err = err
			currentOffset += uint64(r)
			remaining -= uint64(r)
			c <- chnk
		}
	}()
	return c, nil
}

type chunk struct {
	Size      uint64
	Remaining int64
	Content   []byte
	Err       error
}

type pfs0File struct {
	StartOffset uint64
	Size        uint64
	Name        string
}
