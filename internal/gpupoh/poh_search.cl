// PoH search kernels (OpenCL 1.2).
// v1: eval(n)=n*7+13; record smallest n with eval % mod == 0.
// v3: Blake3(prev_ascii || miner_ascii || nonce_u64_le); work = LE uint64(digest[0:8]); work % mod == 0.
// Parity target: CUDA internal/gpupoh/poh_search.cu (same message layout as chain.PohWorkBlake3).
//
// No cl_khr_int64_base_atomics: Mesa rusticl rejects ulong atom_cmpxchg; use int spinlock + plain ulong writes.

// --- v1 algebraic ---
__kernel void poh_search(
	const ulong base,
	const ulong count,
	const ulong mod,
	__global ulong *out_nonce,
	volatile __global int *lock
) {
	size_t tid = get_global_id(0);
	if (tid >= (size_t)count) {
		return;
	}
	ulong n = base + (ulong)tid;
	ulong v = n * 7UL + 13UL;
	if (mod == 0UL) {
		return;
	}
	if ((v % mod) != 0UL) {
		return;
	}
	while (atomic_xchg(lock, 1) != 0) { }
	ulong cur = *out_nonce;
	if (n < cur) {
		*out_nonce = n;
	}
	atomic_xchg(lock, 0);
}

// --- Compact BLAKE3 (hash only, messages <= 1024 bytes) ---
// Based on BLAKE3 portable reference (CC0 / Apache-2.0).

#define BLAKE3_OUT_LEN 32
#define BLAKE3_BLOCK_LEN 64
#define BLAKE3_CHUNK_START (1u << 0)
#define BLAKE3_CHUNK_END (1u << 1)
#define BLAKE3_ROOT (1u << 3)

#define BLAKE3_ROUND(state, msg, i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15) \
	do {                                                                                            \
		g((state), 0, 4, 8, 12, (msg)[i0], (msg)[i1]);                                              \
		g((state), 1, 5, 9, 13, (msg)[i2], (msg)[i3]);                                              \
		g((state), 2, 6, 10, 14, (msg)[i4], (msg)[i5]);                                             \
		g((state), 3, 7, 11, 15, (msg)[i6], (msg)[i7]);                                             \
		g((state), 0, 5, 10, 15, (msg)[i8], (msg)[i9]);                                             \
		g((state), 1, 6, 11, 12, (msg)[i10], (msg)[i11]);                                           \
		g((state), 2, 7, 8, 13, (msg)[i12], (msg)[i13]);                                            \
		g((state), 3, 4, 9, 14, (msg)[i14], (msg)[i15]);                                            \
	} while (0)

#ifndef BLAKE3_NONCES_PER_THREAD
#define BLAKE3_NONCES_PER_THREAD 2
#endif

__constant uint BLAKE3_IV[8] = {
	0x6A09E667u, 0xBB67AE85u, 0x3C6EF372u, 0xA54FF53Au,
	0x510E527Fu, 0x9B05688Cu, 0x1F83D9ABu, 0x5BE0CD19u};

static uint rotr32(uint w, uint c) {
	return (w >> c) | (w << (32u - c));
}

static void g(uint *state, size_t a, size_t b, size_t c, size_t d, uint x, uint y) {
	state[a] = state[a] + state[b] + x;
	state[d] = rotr32(state[d] ^ state[a], 16u);
	state[c] = state[c] + state[d];
	state[b] = rotr32(state[b] ^ state[c], 12u);
	state[a] = state[a] + state[b] + y;
	state[d] = rotr32(state[d] ^ state[a], 8u);
	state[c] = state[c] + state[d];
	state[b] = rotr32(state[b] ^ state[c], 7u);
}

static void load_block_words_le(uint out[16], const uchar block[BLAKE3_BLOCK_LEN]) {
	int i;
	for (i = 0; i < 16; i++) {
		int o = i * 4;
		out[i] = (uint)block[o] | ((uint)block[o + 1] << 8) |
			((uint)block[o + 2] << 16) | ((uint)block[o + 3] << 24);
	}
}

static void compress_pre(uint state[16], const uint cv[8],
	const uchar block[BLAKE3_BLOCK_LEN],
	ulong counter, uint block_len, uint flags) {
	uint block_words[16];
	int i;
	load_block_words_le(block_words, block);
	for (i = 0; i < 8; i++) state[i] = cv[i];
	state[8] = BLAKE3_IV[0];
	state[9] = BLAKE3_IV[1];
	state[10] = BLAKE3_IV[2];
	state[11] = BLAKE3_IV[3];
	state[12] = (uint)counter;
	state[13] = (uint)(counter >> 32);
	state[14] = block_len;
	state[15] = flags;
	BLAKE3_ROUND(state, block_words, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15);
	BLAKE3_ROUND(state, block_words, 2, 6, 3, 10, 7, 0, 4, 13, 1, 11, 12, 5, 9, 14, 15, 8);
	BLAKE3_ROUND(state, block_words, 3, 4, 10, 12, 13, 2, 7, 14, 6, 5, 9, 0, 11, 15, 8, 1);
	BLAKE3_ROUND(state, block_words, 10, 7, 12, 9, 14, 3, 13, 15, 4, 0, 11, 2, 5, 8, 1, 6);
	BLAKE3_ROUND(state, block_words, 12, 13, 9, 11, 15, 10, 14, 8, 7, 2, 5, 3, 0, 1, 6, 4);
	BLAKE3_ROUND(state, block_words, 9, 14, 11, 5, 8, 12, 15, 1, 13, 3, 0, 10, 2, 6, 4, 7);
	BLAKE3_ROUND(state, block_words, 11, 15, 5, 0, 1, 9, 8, 6, 14, 10, 2, 12, 3, 4, 7, 13);
}

static void blake3_hash_one_chunk(const uchar *input, size_t input_len, uchar out[BLAKE3_OUT_LEN]) {
	uint cv[8];
	uchar block[BLAKE3_BLOCK_LEN];
	size_t offset = 0;
	uint flags = BLAKE3_CHUNK_START;
	int i;
	for (i = 0; i < 8; i++) cv[i] = BLAKE3_IV[i];
	while (input_len - offset > BLAKE3_BLOCK_LEN) {
		for (i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = input[offset + (size_t)i];
		{
			uint state[16];
			compress_pre(state, cv, block, 0UL, (uint)BLAKE3_BLOCK_LEN, flags);
			for (i = 0; i < 8; i++) cv[i] = state[i] ^ state[i + 8];
		}
		offset += BLAKE3_BLOCK_LEN;
		flags = 0u;
	}
	{
		size_t rem = input_len - offset;
		uint block_flags;
		uint state[16];
		for (i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = 0;
		for (i = 0; (size_t)i < rem; i++) block[i] = input[offset + (size_t)i];
		block_flags = flags | BLAKE3_CHUNK_END | BLAKE3_ROOT;
		compress_pre(state, cv, block, 0UL, (uint)rem, block_flags);
		for (i = 0; i < 8; i++) {
			uint w = state[i] ^ state[i + 8];
			out[4 * i + 0] = (uchar)(w);
			out[4 * i + 1] = (uchar)(w >> 8);
			out[4 * i + 2] = (uchar)(w >> 16);
			out[4 * i + 3] = (uchar)(w >> 24);
		}
	}
}

static void blake3_hash_from_cv(const uint cv0[8], const uchar *input, size_t input_len,
	uchar out[BLAKE3_OUT_LEN]) {
	uint cv[8];
	uchar block[BLAKE3_BLOCK_LEN];
	size_t offset = 0;
	uint flags = 0u;
	int i;
	for (i = 0; i < 8; i++) cv[i] = cv0[i];
	while (input_len - offset > BLAKE3_BLOCK_LEN) {
		for (i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = input[offset + (size_t)i];
		{
			uint state[16];
			compress_pre(state, cv, block, 0UL, (uint)BLAKE3_BLOCK_LEN, flags);
			for (i = 0; i < 8; i++) cv[i] = state[i] ^ state[i + 8];
		}
		offset += BLAKE3_BLOCK_LEN;
		flags = 0u;
	}
	{
		size_t rem = input_len - offset;
		uint block_flags;
		uint state[16];
		for (i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = 0;
		for (i = 0; (size_t)i < rem; i++) block[i] = input[offset + (size_t)i];
		block_flags = flags | BLAKE3_CHUNK_END | BLAKE3_ROOT;
		compress_pre(state, cv, block, 0UL, (uint)rem, block_flags);
		for (i = 0; i < 8; i++) {
			uint w = state[i] ^ state[i + 8];
			out[4 * i + 0] = (uchar)(w);
			out[4 * i + 1] = (uchar)(w >> 8);
			out[4 * i + 2] = (uchar)(w >> 16);
			out[4 * i + 3] = (uchar)(w >> 24);
		}
	}
}

static ulong blake3_work64_from_cv_words(const uint cv0[8], const uint block_words[16],
	uint block_len) {
	uint state[16];
	uchar block[BLAKE3_BLOCK_LEN];
	int i;
	for (i = 0; i < 16; i++) {
		uint w = block_words[i];
		block[4 * i + 0] = (uchar)(w);
		block[4 * i + 1] = (uchar)(w >> 8);
		block[4 * i + 2] = (uchar)(w >> 16);
		block[4 * i + 3] = (uchar)(w >> 24);
	}
	compress_pre(state, cv0, block, 0UL, block_len, BLAKE3_CHUNK_END | BLAKE3_ROOT);
	{
		uint w0 = state[0] ^ state[8];
		uint w1 = state[1] ^ state[9];
		return (ulong)w0 | ((ulong)w1 << 32);
	}
}

static void put_u64_le_bytes(uint words[16], int byte_off, ulong n) {
	int i;
	for (i = 0; i < 8; i++) {
		int b = byte_off + i;
		int wi = b >> 2;
		int sh = (b & 3) * 8;
		uint mask = 0xFFu << sh;
		words[wi] = (words[wi] & ~mask) | ((uint)((n >> (8 * i)) & 0xFFu) << sh);
	}
}

static int work_divisible_mod(ulong work, ulong inv, ulong lim, uint tz) {
	if (tz > 0u) {
		if ((work & ((1UL << tz) - 1UL)) != 0UL) {
			return 0;
		}
		work >>= tz;
	}
	return (work * inv) <= lim;
}

static void record_min_nonce(__global ulong *out_nonce, volatile __global int *lock, ulong n) {
	while (atomic_xchg(lock, 1) != 0) { }
	{
		ulong cur = *out_nonce;
		if (n < cur) {
			*out_nonce = n;
		}
	}
	atomic_xchg(lock, 0);
}

// prefix: prev_hash_hex_ascii_lower || miner_address_utf8 (no nonce).
// Each work-item appends nonce as 8-byte little-endian and Blake3-hashes.
// When prefix_len >= 64, one local thread compresses the first 64B into local CV;
// others only hash suffix||nonce — same optimization as CUDA poh_search_blake3.
// mod_inv / mod_lim / mod_tz: host prepareBlake3ModDiv(mod).
// share_mod==0 disables share harvest. Otherwise every nonce whose work is
// divisible by share_mod is appended to out_shares (capped by max_shares).
__kernel void poh_search_blake3(
	const ulong base,
	const ulong count,
	const ulong mod,
	const ulong mod_inv,
	const ulong mod_lim,
	const uint mod_tz,
	__global const uchar *prefix,
	const int prefix_len,
	__global ulong *out_nonce,
	volatile __global int *lock,
	const ulong share_mod,
	const ulong share_inv,
	const ulong share_lim,
	const uint share_tz,
	__global uint *out_share_count,
	__global ulong *out_shares,
	const uint max_shares
) {
	__local uchar sprefix[960];
	__local uint scv[8];
	__local uint stail_words[16];
	__local int s_use_cv;
	__local int s_rem;
	int lid = (int)get_local_id(0);
	int lsz = (int)get_local_size(0);
	int i;

	if (prefix_len < 0 || prefix_len > 960 || mod == 0UL) {
		return;
	}
	for (i = lid; i < prefix_len; i += lsz) {
		sprefix[i] = prefix[i];
	}
	barrier(CLK_LOCAL_MEM_FENCE);

	if (lid == 0) {
		s_use_cv = 0;
		s_rem = 0;
		for (i = 0; i < 16; i++) stail_words[i] = 0u;
		if (prefix_len >= BLAKE3_BLOCK_LEN) {
			uint cv[8];
			uchar block[BLAKE3_BLOCK_LEN];
			uint state[16];
			int rem;
			for (i = 0; i < 8; i++) cv[i] = BLAKE3_IV[i];
			for (i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = sprefix[i];
			compress_pre(state, cv, block, 0UL, (uint)BLAKE3_BLOCK_LEN, BLAKE3_CHUNK_START);
			for (i = 0; i < 8; i++) scv[i] = state[i] ^ state[i + 8];
			s_use_cv = 1;
			rem = prefix_len - BLAKE3_BLOCK_LEN;
			s_rem = rem;
			// Word path only when suffix||nonce fits one Blake3 block (rem+8 <= 64).
			if (rem + 8 <= BLAKE3_BLOCK_LEN) {
				for (i = 0; i < rem; i++) {
					int wi = i >> 2;
					int sh = (i & 3) * 8;
					stail_words[wi] |= ((uint)sprefix[BLAKE3_BLOCK_LEN + i]) << sh;
				}
			}
		}
	}
	barrier(CLK_LOCAL_MEM_FENCE);

	{
		size_t tid = get_global_id(0);
		size_t idx0 = tid * (size_t)BLAKE3_NONCES_PER_THREAD;
		int plen = prefix_len;
		int k;
		if (idx0 >= (size_t)count) {
			return;
		}
		if (plen + 8 > 160) {
			return;
		}
		for (k = 0; k < BLAKE3_NONCES_PER_THREAD; k++) {
			size_t idx = idx0 + (size_t)k;
			ulong n;
			ulong work;
			if (idx >= (size_t)count) {
				break;
			}
			n = base + (ulong)idx;
			if (s_use_cv && s_rem + 8 <= BLAKE3_BLOCK_LEN) {
				uint words[16];
				for (i = 0; i < 16; i++) words[i] = stail_words[i];
				put_u64_le_bytes(words, s_rem, n);
				work = blake3_work64_from_cv_words(scv, words, (uint)(s_rem + 8));
			} else if (s_use_cv) {
				int rem = s_rem;
				uchar tail[96];
				uchar digest[32];
				for (i = 0; i < rem; i++) {
					tail[i] = sprefix[BLAKE3_BLOCK_LEN + i];
				}
				tail[rem + 0] = (uchar)(n);
				tail[rem + 1] = (uchar)(n >> 8);
				tail[rem + 2] = (uchar)(n >> 16);
				tail[rem + 3] = (uchar)(n >> 24);
				tail[rem + 4] = (uchar)(n >> 32);
				tail[rem + 5] = (uchar)(n >> 40);
				tail[rem + 6] = (uchar)(n >> 48);
				tail[rem + 7] = (uchar)(n >> 56);
				blake3_hash_from_cv(scv, tail, (size_t)rem + 8, digest);
				work =
					(ulong)digest[0] |
					((ulong)digest[1] << 8) |
					((ulong)digest[2] << 16) |
					((ulong)digest[3] << 24) |
					((ulong)digest[4] << 32) |
					((ulong)digest[5] << 40) |
					((ulong)digest[6] << 48) |
					((ulong)digest[7] << 56);
			} else {
				uchar msg[160];
				uchar digest[32];
				for (i = 0; i < plen; i++) {
					msg[i] = sprefix[i];
				}
				msg[plen + 0] = (uchar)(n);
				msg[plen + 1] = (uchar)(n >> 8);
				msg[plen + 2] = (uchar)(n >> 16);
				msg[plen + 3] = (uchar)(n >> 24);
				msg[plen + 4] = (uchar)(n >> 32);
				msg[plen + 5] = (uchar)(n >> 40);
				msg[plen + 6] = (uchar)(n >> 48);
				msg[plen + 7] = (uchar)(n >> 56);
				blake3_hash_one_chunk(msg, (size_t)plen + 8, digest);
				work =
					(ulong)digest[0] |
					((ulong)digest[1] << 8) |
					((ulong)digest[2] << 16) |
					((ulong)digest[3] << 24) |
					((ulong)digest[4] << 32) |
					((ulong)digest[5] << 40) |
					((ulong)digest[6] << 48) |
					((ulong)digest[7] << 56);
			}
			if (share_mod != 0UL && out_shares != 0 && out_share_count != 0 && max_shares > 0u) {
				if (work_divisible_mod(work, share_inv, share_lim, share_tz)) {
					uint si = atomic_inc(out_share_count);
					if (si < max_shares) {
						out_shares[si] = n;
					}
				}
			}
			if (work_divisible_mod(work, mod_inv, mod_lim, mod_tz)) {
				record_min_nonce(out_nonce, lock, n);
			}
		}
	}
}
