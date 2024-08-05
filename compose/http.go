package compose

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/launchrctl/keyring"
	"github.com/launchrctl/launchr/pkg/cli"
	"github.com/launchrctl/launchr/pkg/log"
)

var (
	errInvalidaFilepath       = errors.New("invalid filepath")
	errNoURL                  = errors.New("invalid url")
	errFailedClose            = errors.New("failed to close stream")
	errRepositoryNotFound     = errors.New("repository not found")
	errAuthenticationRequired = errors.New("authentication required")
	errAuthorizationFailed    = errors.New("authorization failed")
	errHTTPUnknown            = errors.New("unhandled error")
)

var (
	rgxNameFromURL = regexp.MustCompile(`[^\/]+(\/$|$)`)
	rgxArchiveType = regexp.MustCompile(`(zip|tar\.gz)$`)
	rgxPathRoot    = regexp.MustCompile(`^[^\/]*`)
)

type httpDownloader struct{}

func newHTTP() Downloader {
	return &httpDownloader{}
}

// Download implements Downloader.Download interface
func (h *httpDownloader) Download(pkg *Package, targetDir string, kw *keyringWrapper) error {
	url := pkg.GetURL()
	name := rgxNameFromURL.FindString(url)
	if name == "" {
		return errNoURL
	}

	fmt.Println(fmt.Sprintf("http download: " + name))
	fpath := filepath.Clean(filepath.Join(targetDir, name))

	err := os.MkdirAll(targetDir, dirPermissions)
	if err != nil {
		return err
	}

	out, err := os.Create(fpath)
	if err != nil {
		return err
	}

	defer func() {
		if err = out.Close(); err != nil {
			fmt.Println(errFailedClose.Error())
		}
	}()

	client := &http.Client{}
	var resp *http.Response

	errDownloadFailed := fmt.Errorf("failed to download package: %s", name)

	auths := []authorizationMode{authorisationNone, authorisationKeyring, authorisationManual}
	for _, authType := range auths {
		req, errReq := http.NewRequest(http.MethodGet, url, nil)
		if errReq != nil {
			return errReq
		}

		if authType == authorisationNone {
			resp, err = doRequest(client, req)
			if err != nil {
				if errors.Is(err, errAuthenticationRequired) {
					cli.Println("auth required, trying keyring authorisation")
					continue
				}

				log.Debug(err.Error())
				return errDownloadFailed
			}
		}

		if authType == authorisationKeyring {
			ci, errGet := kw.getForURL(url, false)
			if errGet != nil {
				return errGet
			}

			req.SetBasicAuth(ci.Username, ci.Password)
			resp, err = doRequest(client, req)
			if err != nil {
				if errors.Is(err, errAuthorizationFailed) {
					if kw.interactive {
						cli.Println("invalid auth, trying manual authorisation")
						continue
					}
				}

				log.Debug(err.Error())
				return errDownloadFailed
			}
		}

		if authType == authorisationManual {
			ci := keyring.CredentialsItem{}
			ci.URL = url
			ci, errFill := kw.fillCredentials(ci)
			if errFill != nil {
				return errFill
			}

			req.SetBasicAuth(ci.Username, ci.Password)
			resp, err = doRequest(client, req)
			if err != nil {
				log.Debug(err.Error())
				return errDownloadFailed
			}
		}

		break
	}

	defer func() {
		if err = resp.Body.Close(); err != nil {
			fmt.Println(errFailedClose.Error())
		}
	}()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	var archiveRootDir string
	switch at := rgxArchiveType.FindString(name); at {
	case "tar.gz":
		archiveRootDir, err = untar(fpath, targetDir)
	case "zip":
		archiveRootDir, err = unzip(fpath, targetDir)
	default:
		err = fmt.Errorf("not supported archive type: %s", at)
	}

	if err != nil {
		return err
	}

	if archiveRootDir != "" {
		// rename root folder to package name
		return os.Rename(
			filepath.Join(targetDir, archiveRootDir),
			filepath.Join(targetDir, pkg.GetTarget()),
		)
	}

	return nil
}

func doRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	resp, err := client.Do(req)
	if err != nil {
		resp.Body.Close() //nolint
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return resp, nil
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return nil, errAuthenticationRequired
	case http.StatusForbidden:
		return nil, errAuthorizationFailed
	case http.StatusNotFound:
		return nil, errRepositoryNotFound

	default:
		return resp, errHTTPUnknown
	}
}

func untar(fpath, tpath string) (string, error) {
	var rootDir string
	r, err := os.Open(filepath.Clean(fpath))
	if err != nil {
		return rootDir, err
	}

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return rootDir, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()

		switch {

		// if no more files are found return
		case err == io.EOF:
			if rootDir != "" {
				rootDir = rgxPathRoot.FindString(rootDir)
			}

			return rootDir, nil

		// return any other error
		case err != nil:
			return rootDir, err

		// if the header is nil, just skip it (not sure how this happens)
		case header == nil:
			continue
		}

		// the target location where the dir/file should be created
		target, err := sanitizeArchivePath(tpath, header.Name)
		if err != nil {
			return rootDir, errInvalidaFilepath
		}

		if !strings.HasPrefix(target, filepath.Clean(tpath)) {
			return rootDir, errInvalidaFilepath
		}

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			rootDir = header.Name
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0750); err != nil {
					return rootDir, err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(filepath.Clean(target), os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return rootDir, err
			}

			for {
				_, err = io.CopyN(f, tr, 1024)
				if err != nil {
					if err != io.EOF {
						return rootDir, err
					}
					break
				}
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			err = f.Close()
			if err != nil {
				return rootDir, err
			}
		}
	}
}

// Unzip archive
// returns root folder name
func unzip(fpath, tpath string) (string, error) {
	var rootDir string
	archive, err := zip.OpenReader(fpath)
	if err != nil {
		return rootDir, err
	}
	defer archive.Close()

	for _, f := range archive.File {
		filePath, err := sanitizeArchivePath(tpath, f.Name)
		if err != nil || !strings.HasPrefix(filePath, filepath.Clean(tpath)+string(os.PathSeparator)) {
			return rootDir, errInvalidaFilepath
		}
		if f.FileInfo().IsDir() {
			rootDir = f.Name
			err = os.MkdirAll(filePath, os.ModePerm)
			if err != nil {
				return rootDir, err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return rootDir, err
		}

		dstFile, err := os.OpenFile(filepath.Clean(filePath), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return rootDir, err
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return rootDir, err
		}

		for {
			_, err = io.CopyN(dstFile, fileInArchive, 1024)
			if err != nil {
				if err != io.EOF {
					return rootDir, err
				}
				break
			}
		}

		err = dstFile.Close()
		if err != nil {
			return rootDir, err
		}

		err = fileInArchive.Close()

		if err != nil {
			return rootDir, err
		}
	}

	if rootDir != "" {
		rootDir = rgxPathRoot.FindString(rootDir)
	}

	return rootDir, nil
}

func sanitizeArchivePath(d, t string) (v string, err error) {
	v = filepath.Join(d, t)
	if strings.HasPrefix(v, filepath.Clean(d)) {
		return v, nil
	}

	return "", fmt.Errorf("%s: %s", "content filepath is tainted", t)
}
