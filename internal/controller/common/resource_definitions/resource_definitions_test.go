// Copyright The Cryostat Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package resource_definitions_test

import (
	"net/url"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	operatorv1beta2 "github.com/cryostatio/cryostat-operator/api/v1beta2"
	"github.com/cryostatio/cryostat-operator/internal/controller/common/resource_definitions"
	"github.com/cryostatio/cryostat-operator/internal/controller/model"
)

func TestResourceDefinitions(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Resource Definitions Suite")
}

var _ = Describe("newStorageEnvForCoreContainer", func() {
	var (
		cr    *model.CryostatInstance
		specs *resource_definitions.ServiceSpecs
	)

	BeforeEach(func() {
		secretName := "test-storage-secret"
		cr = &model.CryostatInstance{
			Name:             "test-cryostat",
			InstallNamespace: "test-namespace",
			Spec: &operatorv1beta2.CryostatSpec{
				ObjectStorageOptions: &operatorv1beta2.ObjectStorageOptions{
					SecretName: &secretName,
				},
			},
			Status: &operatorv1beta2.CryostatStatus{
				StorageSecret: secretName,
			},
		}

		storageURL, err := url.Parse("http://test-storage:8333")
		Expect(err).ToNot(HaveOccurred())

		specs = &resource_definitions.ServiceSpecs{
			StorageURL: storageURL,
		}
	})

	Context("with managed storage", func() {
		BeforeEach(func() {
			// For managed storage, ObjectStorageOptions should be nil
			cr.Spec.ObjectStorageOptions = nil
			cr.Status.StorageSecret = "test-storage-secret"
		})

		It("should set default presigned transfers enabled", func() {
			envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
			Expect(err).ToNot(HaveOccurred())

			presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
			Expect(presignedTransfersEnv).ToNot(BeNil())
			Expect(presignedTransfersEnv.Value).To(Equal("true"))
		})

		It("should not set presigned downloads env when not specified", func() {
			envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
			Expect(err).ToNot(HaveOccurred())

			presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
			Expect(presignedDownloadsEnv).To(BeNil())
		})

		Context("when DisablePresignedFileTransfers is true", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := true
				// For managed storage with custom settings, Provider can be set but URL should be nil
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						DisablePresignedFileTransfers: &disablePresignedFileTransfers,
						URL:                           nil, // nil URL means managed storage
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should set presigned transfers to false", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("false"))
			})
		})

		Context("when DisablePresignedFileTransfers is false", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := false
				// For managed storage with custom settings, Provider can be set but URL should be nil
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						DisablePresignedFileTransfers: &disablePresignedFileTransfers,
						URL:                           nil, // nil URL means managed storage
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should set presigned transfers to true", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("true"))
			})
		})

		Context("when DisablePresignedDownloads is true", func() {
			BeforeEach(func() {
				disablePresignedDownloads := true
				// For managed storage with custom settings, Provider can be set but URL should be nil
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						DisablePresignedDownloads: &disablePresignedDownloads,
						URL:                       nil, // nil URL means managed storage
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should set presigned downloads to false", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("false"))
			})
		})

		Context("when DisablePresignedDownloads is false", func() {
			BeforeEach(func() {
				disablePresignedDownloads := false
				// For managed storage with custom settings, Provider can be set but URL should be nil
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						DisablePresignedDownloads: &disablePresignedDownloads,
						URL:                       nil, // nil URL means managed storage
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should set presigned downloads to true", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("true"))
			})
		})

		Context("when both DisablePresignedFileTransfers and DisablePresignedDownloads are set", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := true
				disablePresignedDownloads := false
				// For managed storage with custom settings, Provider can be set but URL should be nil
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						DisablePresignedFileTransfers: &disablePresignedFileTransfers,
						DisablePresignedDownloads:     &disablePresignedDownloads,
						URL:                           nil, // nil URL means managed storage
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should set both environment variables correctly", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("false"))

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("true"))
			})
		})
	})

	Context("with external storage", func() {
		BeforeEach(func() {
			url := "https://s3.amazonaws.com"
			region := "us-east-1"
			cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
				Provider: &operatorv1beta2.ObjectStorageProviderOptions{
					URL:    &url,
					Region: &region,
				},
			}
		})

		It("should set default presigned transfers enabled", func() {
			envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
			Expect(err).ToNot(HaveOccurred())

			presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
			Expect(presignedTransfersEnv).ToNot(BeNil())
			Expect(presignedTransfersEnv.Value).To(Equal("true"))
		})

		It("should not set presigned downloads env when not specified", func() {
			envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
			Expect(err).ToNot(HaveOccurred())

			presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
			Expect(presignedDownloadsEnv).To(BeNil())
		})

		Context("when DisablePresignedFileTransfers is true", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := true
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedFileTransfers = &disablePresignedFileTransfers
			})

			It("should set presigned transfers to false", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("false"))
			})
		})

		Context("when DisablePresignedFileTransfers is false", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := false
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedFileTransfers = &disablePresignedFileTransfers
			})

			It("should set presigned transfers to true", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("true"))
			})
		})

		Context("when DisablePresignedDownloads is true", func() {
			BeforeEach(func() {
				disablePresignedDownloads := true
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedDownloads = &disablePresignedDownloads
			})

			It("should set presigned downloads to false", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("false"))
			})
		})

		Context("when DisablePresignedDownloads is false", func() {
			BeforeEach(func() {
				disablePresignedDownloads := false
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedDownloads = &disablePresignedDownloads
			})

			It("should set presigned downloads to true", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("true"))
			})
		})

		Context("when both DisablePresignedFileTransfers and DisablePresignedDownloads are set", func() {
			BeforeEach(func() {
				disablePresignedFileTransfers := true
				disablePresignedDownloads := false
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedFileTransfers = &disablePresignedFileTransfers
				cr.Spec.ObjectStorageOptions.Provider.DisablePresignedDownloads = &disablePresignedDownloads
			})

			It("should set both environment variables correctly", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())

				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("false"))

				presignedDownloadsEnv := findEnvVar(envs, "STORAGE_PRESIGNED_DOWNLOADS_ENABLED")
				Expect(presignedDownloadsEnv).ToNot(BeNil())
				Expect(presignedDownloadsEnv.Value).To(Equal("true"))
			})
		})

		Context("when Provider URL is nil", func() {
			BeforeEach(func() {
				region := "us-east-1"
				cr.Spec.ObjectStorageOptions = &operatorv1beta2.ObjectStorageOptions{
					Provider: &operatorv1beta2.ObjectStorageProviderOptions{
						URL:    nil,
						Region: &region,
					},
				}
				cr.Status.StorageSecret = "test-storage-secret"
			})

			It("should treat it as managed storage and succeed", func() {
				envs, err := resource_definitions.NewStorageEnvForCoreContainer(cr, specs)
				Expect(err).ToNot(HaveOccurred())
				Expect(envs).ToNot(BeEmpty())

				// Should have managed storage settings
				endpointEnv := findEnvVar(envs, "QUARKUS_S3_ENDPOINT_OVERRIDE")
				Expect(endpointEnv).ToNot(BeNil())
				Expect(endpointEnv.Value).To(Equal(specs.StorageURL.String()))

				// Should have default presigned transfers enabled
				presignedTransfersEnv := findEnvVar(envs, "STORAGE_PRESIGNED_TRANSFERS_ENABLED")
				Expect(presignedTransfersEnv).ToNot(BeNil())
				Expect(presignedTransfersEnv.Value).To(Equal("true"))
			})
		})
	})
})

func findEnvVar(envs []corev1.EnvVar, name string) *corev1.EnvVar {
	for i := range envs {
		if envs[i].Name == name {
			return &envs[i]
		}
	}
	return nil
}
