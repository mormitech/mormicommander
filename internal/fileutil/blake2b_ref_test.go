// Reference digests for TestBLAKE2bLongVectors. They were produced once with
// Python's hashlib.blake2b over the i % 251 byte pattern and pasted here, so
// the test does not need Python; the lengths 128, 129 and 1000 cover an
// exact block, one byte past a block, and many blocks.

package fileutil

const (
	blake2bRef128  = "2319e3789c47e2daa5fe807f61bec2a1a6537fa03f19ff32e87eecbfd64b7e0e8ccff439ac333b040f19b0c4ddd11a61e24ac1fe0f10a039806c5dcc0da3d115"
	blake2bRef129  = "f59711d44a031d5f97a9413c065d1e614c417ede998590325f49bad2fd444d3e4418be19aec4e11449ac1a57207898bc57d76a1bcf3566292c20c683a5c4648f"
	blake2bRef1000 = "c11e1c0340bd7e5a1b275f1230c962fad215ecb1391486e74e31b960a2f2996381a5fad092da06841d5f26e38f6ecfeaf441acbcd1c2de61aef121e7927175f5"
)
