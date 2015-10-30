/*
 ************************************************************
 lzhuf.c
 written by Haruyasu Yoshizaki 11/20/1988
 some minor changes 4/6/1989
 comments translated by Haruhiko Okumura 4/7/1989
 Adapted to Jnos 1.10h by Jack Snodgrass, KF5MG, 12/19/94
 ************************************************************
 *
 * 22Dec2005, Maiko, Replaced malloc() with mallocw() instead !
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#ifdef MSDOS
#include <alloc.h>
#endif

#include <time.h>
#include "global.h"

#if defined(LZHUF) || defined(FBBCMP)
#include "proc.h"
#include "socket.h"
#include "timer.h"
#include "usock.h"
#include "netuser.h"
#include "session.h"

#include "lzhuf.h"

#ifdef	B2F
/* 24Mar2008, Maiko (VE4KLM), for reference to mbx structure */
#include "mailbox.h"
#endif

#ifdef XMS
#include "xms.h"
#endif
#ifdef UNIX
#define printf tcmdprintf
#endif

#ifdef HFDD
extern int hfdd_debug;
#endif

/* see error codes defined in fbbfwd.c */
#define FBBerror(x,y) { tprintf("*** Protocole Error (%d)\n", x); log(y,"lzhuf: detected FBB protocol error %d", x); }

static char EarlyDisconnect[] = "lzhuf: unexpected disconnect";

/* return 1 if allocations succeeded, else
   return 0 after freeing partial allocations
*/
int AllocDataBuffers(struct fwd *f) {

   if ((f->lzhuf = calloc(sizeof(struct lzhufstruct),1)) == NULL) return 0;
   if ((f->tmpBuffer = mallocw(260)) == NULL) {
       free (f->lzhuf);
       f->lzhuf = NULL;
       return 0;
   }

#ifdef LZHDEBUG
   if(Current->output == Command->output)
      printf("Getting %d bytes of storage.\n", sizeof(struct lzhufdata));
#endif

   f->lzhuf->data_type = 0;        // 0 means one big buffer (UMB or not)
#ifdef XMS
   f->lzhuf->data = (struct lzhufdata *)mallocUMB(sizeof(struct lzhufdata));
   if (f->lzhuf->data == NULL)  /* err, or not enough paras avail, so use malloc */
#endif
   f->lzhuf->data = (struct lzhufdata *)mallocw(sizeof(struct lzhufdata));
   if(f->lzhuf->data == (struct lzhufdata *)NULL) {  /* can't get one big buffer, try in pieces */
      f->lzhuf->data_type = 1;        // 1 means small buffers + lower memory.
      f->lzhuf->dad       = mallocw((N + 1) * sizeof(int));
      f->lzhuf->lson      = mallocw((N + 1) * sizeof(int));
      f->lzhuf->rson      = mallocw((N + 257) * sizeof(int));
      f->lzhuf->text_buf  = mallocw((N + F - 1) * sizeof(unsigned char));
      f->lzhuf->freq      = mallocw((T + 1) * sizeof(unsigned));
      f->lzhuf->prnt      = mallocw((T + N_CHAR) * sizeof(int));
      f->lzhuf->son       = mallocw((T) * sizeof(int));

      if (!f->lzhuf->dad || !f->lzhuf->lson || !f->lzhuf->rson ||
          !f->lzhuf->text_buf || !f->lzhuf->freq || !f->lzhuf->prnt ||
          !f->lzhuf->son) {  /* not enough mem for all allocations */
              FreeDataBuffers(f);
              return 0;
          }

   } else {
      // point pointers to correct spot in large buffer.
      f->lzhuf->dad       = f->lzhuf->data->dad;
      f->lzhuf->rson      = f->lzhuf->data->rson;
      f->lzhuf->lson      = f->lzhuf->data->lson;
      f->lzhuf->text_buf  = f->lzhuf->data->text_buf;
      f->lzhuf->freq      = f->lzhuf->data->freq;
      f->lzhuf->prnt      = f->lzhuf->data->prnt;
      f->lzhuf->son       = f->lzhuf->data->son;
   }
   return 1;    /* all OK */
}

void FreeDataBuffers(struct fwd *f) {

   free(f->tmpBuffer);

   if(f->lzhuf->data_type == 1) {
      // Free lower memory blocks.
      free(f->lzhuf->dad);
      free(f->lzhuf->lson);
      free(f->lzhuf->rson);
      free(f->lzhuf->text_buf);
      free(f->lzhuf->freq);
      free(f->lzhuf->prnt);
      free(f->lzhuf->son);
   } else
      free(f->lzhuf->data);
   free(f->lzhuf);
}

#ifdef B2F

/*
 * 16Apr2008, Maiko (VE4KLM), After analyzing my raw data, it seems that
 * Airmail and Winlink 2000 are using the Xmodem variation of CRC-CCITT
 * to do checksums. This is a bit different from our FCS table values.
 *
 * YES !!! Airmail did a successfull decode of my SEND_YAPP !!!
 */

unsigned short crc16tab[256] = {
    0x0000,  0x1021,  0x2042,  0x3063,  0x4084,  0x50a5,  0x60c6,  0x70e7,
    0x8108,  0x9129,  0xa14a,  0xb16b,  0xc18c,  0xd1ad,  0xe1ce,  0xf1ef,
    0x1231,  0x0210,  0x3273,  0x2252,  0x52b5,  0x4294,  0x72f7,  0x62d6,
    0x9339,  0x8318,  0xb37b,  0xa35a,  0xd3bd,  0xc39c,  0xf3ff,  0xe3de,
    0x2462,  0x3443,  0x0420,  0x1401,  0x64e6,  0x74c7,  0x44a4,  0x5485,
    0xa56a,  0xb54b,  0x8528,  0x9509,  0xe5ee,  0xf5cf,  0xc5ac,  0xd58d,
    0x3653,  0x2672,  0x1611,  0x0630,  0x76d7,  0x66f6,  0x5695,  0x46b4,
    0xb75b,  0xa77a,  0x9719,  0x8738,  0xf7df,  0xe7fe,  0xd79d,  0xc7bc,
    0x48c4,  0x58e5,  0x6886,  0x78a7,  0x0840,  0x1861,  0x2802,  0x3823,
    0xc9cc,  0xd9ed,  0xe98e,  0xf9af,  0x8948,  0x9969,  0xa90a,  0xb92b,
    0x5af5,  0x4ad4,  0x7ab7,  0x6a96,  0x1a71,  0x0a50,  0x3a33,  0x2a12,
    0xdbfd,  0xcbdc,  0xfbbf,  0xeb9e,  0x9b79,  0x8b58,  0xbb3b,  0xab1a,
    0x6ca6,  0x7c87,  0x4ce4,  0x5cc5,  0x2c22,  0x3c03,  0x0c60,  0x1c41,
    0xedae,  0xfd8f,  0xcdec,  0xddcd,  0xad2a,  0xbd0b,  0x8d68,  0x9d49,
    0x7e97,  0x6eb6,  0x5ed5,  0x4ef4,  0x3e13,  0x2e32,  0x1e51,  0x0e70,
    0xff9f,  0xefbe,  0xdfdd,  0xcffc,  0xbf1b,  0xaf3a,  0x9f59,  0x8f78,
    0x9188,  0x81a9,  0xb1ca,  0xa1eb,  0xd10c,  0xc12d,  0xf14e,  0xe16f,
    0x1080,  0x00a1,  0x30c2,  0x20e3,  0x5004,  0x4025,  0x7046,  0x6067,
    0x83b9,  0x9398,  0xa3fb,  0xb3da,  0xc33d,  0xd31c,  0xe37f,  0xf35e,
    0x02b1,  0x1290,  0x22f3,  0x32d2,  0x4235,  0x5214,  0x6277,  0x7256,
    0xb5ea,  0xa5cb,  0x95a8,  0x8589,  0xf56e,  0xe54f,  0xd52c,  0xc50d,
    0x34e2,  0x24c3,  0x14a0,  0x0481,  0x7466,  0x6447,  0x5424,  0x4405,
    0xa7db,  0xb7fa,  0x8799,  0x97b8,  0xe75f,  0xf77e,  0xc71d,  0xd73c,
    0x26d3,  0x36f2,  0x0691,  0x16b0,  0x6657,  0x7676,  0x4615,  0x5634,
    0xd94c,  0xc96d,  0xf90e,  0xe92f,  0x99c8,  0x89e9,  0xb98a,  0xa9ab,
    0x5844,  0x4865,  0x7806,  0x6827,  0x18c0,  0x08e1,  0x3882,  0x28a3,
    0xcb7d,  0xdb5c,  0xeb3f,  0xfb1e,  0x8bf9,  0x9bd8,  0xabbb,  0xbb9a,
    0x4a75,  0x5a54,  0x6a37,  0x7a16,  0x0af1,  0x1ad0,  0x2ab3,  0x3a92,
    0xfd2e,  0xed0f,  0xdd6c,  0xcd4d,  0xbdaa,  0xad8b,  0x9de8,  0x8dc9,
    0x7c26,  0x6c07,  0x5c64,  0x4c45,  0x3ca2,  0x2c83,  0x1ce0,  0x0cc1,
    0xef1f,  0xff3e,  0xcf5d,  0xdf7c,  0xaf9b,  0xbfba,  0x8fd9,  0x9ff8,
    0x6e17,  0x7e36,  0x4e55,  0x5e74,  0x2e93,  0x3eb2,  0x0ed1,  0x1ef0
};

#define UPDCRC16(cp, crc) (crc16tab[((crc >> 8) & 255)] ^ (crc << 8) ^ cp)

#endif

/* 23Apr2008, Maiko (VE4KLM), Added flag for b2f considerations */
int  Encode    (int, char *, char *, struct lzhufstruct *, int);

/* 24Mar2008, Maiko (VE4KLM), Added flag for b2f considerations */
int  Decode    (int, char *, char *, struct lzhufstruct *, int);

static int  GetBit    (struct lzhufstruct *);
#ifdef MSDOS
static int  GetByte   (struct lzhufstruct *);
#else
static unsigned short  GetByte   (struct lzhufstruct *);
#endif
static void Putcode   (struct lzhufstruct *, int, unsigned);
static void EncodeEnd (struct lzhufstruct *);
static int  DecodeChar(struct lzhufstruct *);
static int  recvbuf   (int,char *,int,int32);

/********** LZSS compression **********/

static void InitTree(struct lzhufstruct *lzhuf)  /* initialize trees */
{
   int  i;

   for (i = N + 1; i <= N + 256; i++)
       lzhuf->rson[i] = NIL;                  /* root */
   for (i = 0; i < N; i++)
       lzhuf->dad[i] = NIL;                   /* node */
}

static void InsertNode(struct lzhufstruct *lzhuf, int r)  /* insert to tree */
{
   int  i, p, cmp;
   unsigned char  *key;
   unsigned int c;

   cmp = 1;
   key = &lzhuf->text_buf[r];
   p = N + 1 + key[0];
   lzhuf->rson[r] = lzhuf->lson[r] = NIL;
   lzhuf->match_length = 0;
   for(;;) {
      if(cmp >= 0) {
         if(lzhuf->rson[p] != NIL)
            p = lzhuf->rson[p];
         else {
            lzhuf->rson[p] = r;
            lzhuf->dad[r] = p;
            return;
         }
      } else {
         if(lzhuf->lson[p] != NIL)
            p = lzhuf->lson[p];
         else {
            lzhuf->lson[p] = r;
            lzhuf->dad[r] = p;
            return;
         }
      }
      for(i = 1; i < F; i++)
         if((cmp = key[i] - lzhuf->text_buf[p + i]) != 0)
            break;
      if(i > THRESHOLD) {
         if(i > lzhuf->match_length) {
            lzhuf->match_position = ((r - p) & (N - 1)) - 1;
            if((lzhuf->match_length = i) >= F)
               break;
         }
         if(i == lzhuf->match_length) {
            if((int)(c = ((r - p) & (N - 1)) - 1) < lzhuf->match_position) {
               lzhuf->match_position = c;
            }
         }
      }
   }
   lzhuf->dad[r] = lzhuf->dad[p];
   lzhuf->lson[r] = lzhuf->lson[p];
   lzhuf->rson[r] = lzhuf->rson[p];
   lzhuf->dad[lzhuf->lson[p]] = r;
   lzhuf->dad[lzhuf->rson[p]] = r;
   if(lzhuf->rson[lzhuf->dad[p]] == p)
      lzhuf->rson[lzhuf->dad[p]] = r;
   else
      lzhuf->lson[lzhuf->dad[p]] = r;
   lzhuf->dad[p] = NIL;  /* remove p */
}

static void DeleteNode(struct lzhufstruct *lzhuf, int p)  /* remove from tree */
{
   int  q;

   if(lzhuf->dad[p] == NIL)
      return;                 /* not registered */
   if(lzhuf->rson[p] == NIL)
      q = lzhuf->lson[p];
   else
   if(lzhuf->lson[p] == NIL)
      q = lzhuf->rson[p];
   else {
      q = lzhuf->lson[p];
      if(lzhuf->rson[q] != NIL) {
         do {
            q = lzhuf->rson[q];
         } while (lzhuf->rson[q] != NIL);
         lzhuf->rson[lzhuf->dad[q]] = lzhuf->lson[q];
         lzhuf->dad[lzhuf->lson[q]] = lzhuf->dad[q];
         lzhuf->lson[q] = lzhuf->lson[p];
         lzhuf->dad[lzhuf->lson[p]] = q;
      }
      lzhuf->rson[q] = lzhuf->rson[p];
      lzhuf->dad[lzhuf->rson[p]] = q;
   }
   lzhuf->dad[q] = lzhuf->dad[p];
   if(lzhuf->rson[lzhuf->dad[p]] == p)
      lzhuf->rson[lzhuf->dad[p]] = q;
   else
      lzhuf->lson[lzhuf->dad[p]] = q;
   lzhuf->dad[p] = NIL;
}

/* Huffman coding */

/* table for encoding and decoding the upper 6 bits of position */

/* for encoding */
static uchar p_len[64] = {
        0x03, 0x04, 0x04, 0x04, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
        0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08
};

static uchar p_code[64] = {
        0x00, 0x20, 0x30, 0x40, 0x50, 0x58, 0x60, 0x68,
        0x70, 0x78, 0x80, 0x88, 0x90, 0x94, 0x98, 0x9C,
        0xA0, 0xA4, 0xA8, 0xAC, 0xB0, 0xB4, 0xB8, 0xBC,
        0xC0, 0xC2, 0xC4, 0xC6, 0xC8, 0xCA, 0xCC, 0xCE,
        0xD0, 0xD2, 0xD4, 0xD6, 0xD8, 0xDA, 0xDC, 0xDE,
        0xE0, 0xE2, 0xE4, 0xE6, 0xE8, 0xEA, 0xEC, 0xEE,
        0xF0, 0xF1, 0xF2, 0xF3, 0xF4, 0xF5, 0xF6, 0xF7,
        0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xFF
};

/* for decoding */
static uchar d_code[256] = {
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
        0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01,
        0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
        0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
        0x09, 0x09, 0x09, 0x09, 0x09, 0x09, 0x09, 0x09,
        0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A, 0x0A,
        0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B,
        0x0C, 0x0C, 0x0C, 0x0C, 0x0D, 0x0D, 0x0D, 0x0D,
        0x0E, 0x0E, 0x0E, 0x0E, 0x0F, 0x0F, 0x0F, 0x0F,
        0x10, 0x10, 0x10, 0x10, 0x11, 0x11, 0x11, 0x11,
        0x12, 0x12, 0x12, 0x12, 0x13, 0x13, 0x13, 0x13,
        0x14, 0x14, 0x14, 0x14, 0x15, 0x15, 0x15, 0x15,
        0x16, 0x16, 0x16, 0x16, 0x17, 0x17, 0x17, 0x17,
        0x18, 0x18, 0x19, 0x19, 0x1A, 0x1A, 0x1B, 0x1B,
        0x1C, 0x1C, 0x1D, 0x1D, 0x1E, 0x1E, 0x1F, 0x1F,
        0x20, 0x20, 0x21, 0x21, 0x22, 0x22, 0x23, 0x23,
        0x24, 0x24, 0x25, 0x25, 0x26, 0x26, 0x27, 0x27,
        0x28, 0x28, 0x29, 0x29, 0x2A, 0x2A, 0x2B, 0x2B,
        0x2C, 0x2C, 0x2D, 0x2D, 0x2E, 0x2E, 0x2F, 0x2F,
        0x30, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37,
        0x38, 0x39, 0x3A, 0x3B, 0x3C, 0x3D, 0x3E, 0x3F,
};

static uchar d_len[256] = {
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03, 0x03,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
        0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
        0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
};

#ifdef MSDOS
static int GetBit(struct lzhufstruct *lzhuf)        /* get one bit */
{
   int i;

   while(lzhuf->getlen <= 8) {
      if((i = getc(lzhuf->iFile)) < 0) i = 0;
      lzhuf->getbuf |= i << (8 - lzhuf->getlen);
      lzhuf->getlen += 8;
   }
   i = lzhuf->getbuf;
   lzhuf->getbuf <<= 1;
   lzhuf->getlen--;
   return (i < 0);
}

static int GetByte(struct lzhufstruct *lzhuf)       /* get one byte */
{
   unsigned i;

   while(lzhuf->getlen <= 8) {
      if((i = getc(lzhuf->iFile)) == (unsigned)-1) i = 0;
      lzhuf->getbuf |= i << (8 - lzhuf->getlen);
      lzhuf->getlen += 8;
   }
   i = lzhuf->getbuf;
   lzhuf->getbuf <<= 8;
   lzhuf->getlen -= 8;
   return i >> 8;
}
#else	/* alternate from KO4KS, better for non-Borland compilers */
static int GetBit(struct lzhufstruct *lzhuf)        /* get one bit */
{
   register unsigned i;
   register unsigned dx = lzhuf->getbuf;
   register unsigned char glen = lzhuf->getlen;
   
   while(glen <= 8) {
      i = getc(lzhuf->iFile);
      if ((int)i < 0)
	   i = 0;
      dx |= i << (8 - glen);
      glen += 8;
   }
   lzhuf->getbuf = dx << 1;
   lzhuf->getlen = glen - 1;
   return (dx & 0x8000) ? 1 : 0;
}


static unsigned short GetByte(struct lzhufstruct *lzhuf)       /* get one byte */
{
   register unsigned i;
   register unsigned dx = lzhuf->getbuf;
   register unsigned char glen = lzhuf->getlen;

   while(glen <= 8) {
      i = getc(lzhuf->iFile);
      if ((int)i < 0)
	   i = 0;
      dx |= i << (8 - glen);
      glen += 8;
   }
   lzhuf->getbuf = dx << 8;
   lzhuf->getlen = glen - 8;
   return (dx >> 8) & 0xff;
}
#endif

static void Putcode(struct lzhufstruct *lzhuf, int l, unsigned c)         /* output c bits of code */
{
   lzhuf->putbuf |= c >> lzhuf->putlen;
   if((lzhuf->putlen += l) >= 8) {
      if(putc(lzhuf->putbuf >> 8, lzhuf->oFile) == EOF) {
         return;
      }
      if((lzhuf->putlen -= 8) >= 8) {
         if(putc(lzhuf->putbuf, lzhuf->oFile) == EOF) {
            return;
         }
         lzhuf->codesize += 2;
         lzhuf->putlen -= 8;
         lzhuf->putbuf = c << (l - lzhuf->putlen);
      } else {
         lzhuf->putbuf <<= 8;
         lzhuf->codesize++;
      }
   }
}


/* initialization of tree */

static void StartHuff(struct lzhufstruct *lzhuf)
{
   int i, j;

   for(i = 0; i < N_CHAR; i++) {
       lzhuf->freq[i] = 1;
       lzhuf->son[i] = i + T;
       lzhuf->prnt[i + T] = i;
   }
   i = 0; j = N_CHAR;
   while(j <= R) {
       lzhuf->freq[j] = lzhuf->freq[i] + lzhuf->freq[i + 1];
       lzhuf->son[j] = i;
       lzhuf->prnt[i] = lzhuf->prnt[i + 1] = j;
       i += 2; j++;
   }
   lzhuf->freq[T] = 0xffff;
   lzhuf->prnt[R] = 0;
}


/* reconstruction of tree */

static void reconst(struct lzhufstruct *lzhuf)
{
   int i, j, k;
   unsigned first, last;

   /* collect leaf nodes in the first half of the table */
   /* and replace the freq by (freq + 1) / 2. */
   j = 0;
   for(i = 0; i < T; i++) {
       if(lzhuf->son[i] >= T) {
          lzhuf->freq[j] = (lzhuf->freq[i] + 1) / 2;
          lzhuf->son[j] = lzhuf->son[i];
          j++;
       }
   }
   /* begin constructing tree by connecting sons */
   for(i = 0, j = N_CHAR; j < T; i += 2, j++) {
       k = i + 1;
       first = lzhuf->freq[j] = lzhuf->freq[i] + lzhuf->freq[k];
       for (k = j - 1; first < lzhuf->freq[k]; k--);
       k++;
       last = (j - k) * sizeof(unsigned int);  /* # BYTES to move right */
       memmove(&lzhuf->freq[k + 1], &lzhuf->freq[k], last);
       lzhuf->freq[k] = first;
       memmove(&lzhuf->son[k + 1], &lzhuf->son[k], last);
       lzhuf->son[k] = i;
   }
   /* connect prnt */
   for(i = 0; i < T; i++) {
       if((k = lzhuf->son[i]) >= T) {
          lzhuf->prnt[k] = i;
       } else {
          lzhuf->prnt[k] = lzhuf->prnt[k + 1] = i;
       }
   }
}


/* increment frequency of given code by one, and update tree */

static void update(struct lzhufstruct *lzhuf, int c)
{
   int i, j, k, l;

   if(lzhuf->freq[R] == MAX_FREQ) {
      reconst(lzhuf);
   }
   c = lzhuf->prnt[c + T];
   do {
      k = ++lzhuf->freq[c];

      /* if the order is disturbed, exchange nodes */
      if (k > (int)lzhuf->freq[l = c + 1]) {
         while (k > (int)lzhuf->freq[++l]);
         l--;
         lzhuf->freq[c] = lzhuf->freq[l];
         lzhuf->freq[l] = k;

         i = lzhuf->son[c];
         lzhuf->prnt[i] = l;
         if (i < T) lzhuf->prnt[i + 1] = l;

         j = lzhuf->son[l];
         lzhuf->son[l] = i;

         lzhuf->prnt[j] = c;
         if (j < T) lzhuf->prnt[j + 1] = c;
         lzhuf->son[c] = j;

         c = l;
      }
   } while ((c = lzhuf->prnt[c]) != 0);   /* repeat up to root */
}

static void EncodeChar(struct lzhufstruct *lzhuf, unsigned c)
{
   unsigned i;
   int j, k;

   i = 0;
   j = 0;
   k = lzhuf->prnt[c + T];

   /* travel from leaf to root */
   do {
      i >>= 1;

      /* if node's address is odd-numbered, choose bigger brother node */
      if (k & 1) i += 0x8000;

      j++;
   } while ((k = lzhuf->prnt[k]) != R);
   Putcode(lzhuf, j, i);
   lzhuf->code = i;
   lzhuf->len = j;
   update(lzhuf, c);
}

static void EncodePosition(struct lzhufstruct *lzhuf, unsigned c)
{
   unsigned i;

   /* output upper 6 bits by table lookup */
   i = c >> 6;
   Putcode(lzhuf, p_len[i], (unsigned)p_code[i] << 8);

   /* output lower 6 bits verbatim */
   Putcode(lzhuf, 6, (c & 0x3f) << 10);
}

static void EncodeEnd(struct lzhufstruct *lzhuf)
{
   if(lzhuf->putlen) {
      if(putc(lzhuf->putbuf >> 8, lzhuf->oFile) == EOF) {
         return;
      }
      lzhuf->codesize++;
   }
}

static int DecodeChar(struct lzhufstruct *lzhuf)
{
   unsigned c;

   c = lzhuf->son[R];

   /* travel from root to leaf, */
   /* choosing the smaller child node (son[]) if the read bit is 0, */
   /* the bigger (son[]+1} if 1 */
   while (c < T) {
       c += GetBit(lzhuf);
       c = lzhuf->son[c];
   }
   c -= T;
   update(lzhuf, c);
   return c;
}

static int DecodePosition(struct lzhufstruct *lzhuf)
{
   unsigned i, j, c;

   /* recover upper 6 bits from table */
   i = GetByte(lzhuf);
   c = (unsigned)d_code[i] << 6;
   j = d_len[i];

   /* read lower 6 bits verbatim */
   j -= 2;
   while (j--) {
      i = (i << 1) + GetBit(lzhuf);
   }
   return c | (i & 0x3f);
}

/* 29Dec2004, Replaces GOTO 'errxit' labels */
static int do_errxit (int i, struct lzhufstruct *lzhuf)
{
      i=errno;
      if (!i) i--;  /* some non-zero value */
      fclose(lzhuf->iFile);
      fclose(lzhuf->oFile);
      return i;
}

/* compression */

#ifdef	B2F
#define	EOMODE "wb+"
#else
#define	EOMODE "wb"
#endif

/* Now returns 0 if encodes OK, else errno or -1 for failure to encode */
/* 23Apr2008, Maiko (VE4KLM), Added flag for b2f considerations */
int Encode (int usock, char *iFile, char *oFile, struct lzhufstruct *lzhuf, int  b2f)
{
   int i = 0, c, len, r, s, last_match_length;

   unsigned long int  filesize   = 0;

   int32 fbb_filesize = 0;	/* 10Oct2009, Maiko, Very important - 4 bytes ! */

#ifdef B2F
	unsigned short crc;
#endif

#ifdef LZHDEBUG
   printf("Encoding %s into %s\n", iFile, oFile);
#endif

   // Open input and output files.
   if ( ((lzhuf->iFile = fopen(iFile, "rb")) == NULLFILE)
   || ((lzhuf->oFile = fopen(oFile, EOMODE)) == NULLFILE) )
	return (do_errxit (i, lzhuf));

   fseek(lzhuf->iFile, 0L, 2);
   if ((filesize = ftell(lzhuf->iFile)) == 0)
	return (do_errxit (i, lzhuf));

#ifdef B2F
	if (b2f)
	{
		/*
		 * B2F puts a CRC in front of the regular FBB compressed data, so
		 * just assign a couple of bytes for now to take up space. Then we
		 * update it later when we actually have a valid CRC to put in.
		 * 16Apr2008, Maiko (VE4KLM)
		 */
   		if (fwrite (&crc, sizeof(crc), 1, lzhuf->oFile) < 1)
			return (do_errxit (i, lzhuf));
	}
#endif

	/*
	 * 10Oct2009, Maiko, For 64 bit compatibility, it is very important
	 * to note that the FBB length from the file is 4 bytes, previously
	 * using filesize (which is a long) was causing the code to use 8,
	 * which messes everything up, puts JNOS into an intense loop, etc.
	 */ 

	fbb_filesize = (int32)filesize;

   /* output size of text */
   if(fwrite(&fbb_filesize, sizeof(fbb_filesize), 1, lzhuf->oFile) < 1) {
	return (do_errxit (i, lzhuf));
   }
   rewind(lzhuf->iFile);

   lzhuf->iFileSize = filesize;
   filesize = 0;                   /* rewind and re-read */
   StartHuff(lzhuf);
   InitTree(lzhuf);
   s = 0;
   r = N - F;
   for(i = s; i < r; i++)
       lzhuf->text_buf[i] = ' ';
   for(len = 0; len < F && (c = getc(lzhuf->iFile)) != EOF; len++)
       lzhuf->text_buf[r + len] = c;
   filesize = len;
   for(i = 1; i <= F; i++)
       InsertNode(lzhuf, r - i);
   InsertNode(lzhuf, r);
   do {
      pwait(NULL);
      if(lzhuf->match_length > len)
         lzhuf->match_length = len;
      if(lzhuf->match_length <= THRESHOLD) {
         lzhuf->match_length = 1;
         EncodeChar(lzhuf,lzhuf->text_buf[r]);
      } else {
         EncodeChar(lzhuf,255 - THRESHOLD + lzhuf->match_length);
         EncodePosition(lzhuf,lzhuf->match_position);
      }
      last_match_length = lzhuf->match_length;
      for(i = 0; i < last_match_length && (c = getc(lzhuf->iFile)) != EOF; i++) {
         DeleteNode(lzhuf, s);
         lzhuf->text_buf[s] = c;
         if(s < F - 1)
            lzhuf->text_buf[s + N] = c;
         s = (s + 1) & (N - 1);
         r = (r + 1) & (N - 1);
         InsertNode(lzhuf, r);
      }
      while(i++ < last_match_length) {
         DeleteNode(lzhuf, s);
         s = (s + 1) & (N - 1);
         r = (r + 1) & (N - 1);
         if (--len) InsertNode(lzhuf, r);
      }
   } while (len > 0);
   EncodeEnd(lzhuf);
   fclose(lzhuf->iFile);
#ifdef B2F
	if (b2f)
	{

		int i, cnt = 0;

		/*
		 * If B2F, we need a valid CRC value in the first 2 bytes of the msg,
		 * so before we close the outfile file we need to calculate the CRC,
		 * then rewrite the first 2 bytes (place keepers), then close file.
		 * 16Apr2008, Maiko (VE4KLM)
		 */
		fseek (lzhuf->oFile, 2L, SEEK_SET);	/* don't include place keepers */
		crc = 0;
		/* log (-1, "calculate crc"); */
		while (1)
		{
			if ((i = getc (lzhuf->oFile)) < 0)
				break;
			crc = UPDCRC16 (i, crc);
			cnt++;
		}
		crc = UPDCRC16(0,crc);
		crc = UPDCRC16(0,crc);

		/* log (-1, "crc %d iterations %d", crc, cnt); */

		fseek (lzhuf->oFile, 0L, SEEK_SET);	/* now we can rewrite the CRC */
   		if (fwrite (&crc, sizeof(crc), 1, lzhuf->oFile) < 1)
			return (do_errxit (i, lzhuf));
	}
#endif
   fclose(lzhuf->oFile);

	if (lzhuf->iFileSize > 0)
	{
		log (usock, "lzhuf compress %ld/%ld = %ld percent",
			lzhuf->codesize, lzhuf->iFileSize,
				(lzhuf->iFileSize - lzhuf->codesize) * 100L / lzhuf->iFileSize);
	}

#ifdef LZHSTAT
   if(lzhuf->iFileSize == 0)
      lzhuf->iFileSize  = 1;
   if(Current->output == Command->output)
      printf("lzhuf Compress: %ld/%ld = %ld%%\n",
              lzhuf->codesize, lzhuf->iFileSize,
             (lzhuf->iFileSize - lzhuf->codesize) * 100L / lzhuf->iFileSize);
#endif /* LZHSTAT */
   return 0;
}

/* Now returns 0 if decodes OK, else errno or -1 if decode fails */
/* 24Mar2008, Maiko (VE4KLM), Added flag for b2f considerations */
int Decode(int usock, char* iFile, char* oFile, struct lzhufstruct *lzhuf, int b2f)
{
   int  i = 0;
   int  j = 0;
   int  k = 0;
   int  r = 0;
   int  c = 0;

   int32 fbb_filesize = 0;	/* 10Oct2009, Maiko, Very important - 4 bytes ! */

   unsigned long int  count      = 0;
            long int  filesize   = 0;

   /* log (-1, "Decode (%s) (%s) B2F %d", iFile, oFile, b2f); */

   // Open input and output files.
   if ( ((lzhuf->iFile = fopen(iFile, "rb")) == NULLFILE)
   || ((lzhuf->oFile = fopen(oFile, "wb")) == NULLFILE) )
	return (do_errxit (i, lzhuf));

   fseek(lzhuf->iFile, 0L, 2);

   if ((filesize = ftell(lzhuf->iFile)) == 0)
	return (do_errxit (i, lzhuf));
   lzhuf->iFileSize = filesize;

   /* log (-1, "input file size %ld", filesize); */

   rewind(lzhuf->iFile);

#ifdef B2F
	if (b2f)
	{
		int16 crc;
		/*
		 * B2F puts a CRC in front of the regular FBB compressed data, so
		 * skip the 2 bytes for now, - implement it later of course.
		 * 24Mar2008, Maiko (VE4KLM)
		 * 16Apr2008
		 */
   		if (fread (&crc, sizeof(crc), 1, lzhuf->iFile) < 1)
			log (-1, "could not read crc value");
		/*
		else
			log (-1, "incoming crc %d", crc);
		*/

		/* 16Apr2008, Thanks to a cool website that lets you give hex
		 * data (from the temporary fwding files I got the info), and
		 * then give you the checksums for the varying CRC methods, I
		 * have determined that WL2K and Airmail use CRC-CCITT (xmodem)
		 * method, which differs from our fcstab method ...
		 */
	}
#endif

	/*
	 * 10Oct2009, Maiko, For 64 bit compatibility, it is very important
	 * to note that the FBB length from the file is 4 bytes, previously
	 * using filesize (which is a long) was causing the code to use 8,
	 * which messes everything up, puts JNOS into an intense loop, etc.
	 */ 
   if((fread(&fbb_filesize, sizeof(fbb_filesize), 1, lzhuf->iFile) < 1)
   || (fbb_filesize == 0)) {
      i=errno;
      if (!i) i--;  /* some non-zero value */
      fclose(lzhuf->iFile);
      fclose(lzhuf->oFile);
      return i;
   }

	filesize = (long)fbb_filesize;

	/* log (-1, "fbb_filesize %d, filesize %ld", fbb_filesize, filesize); */

#ifdef B2F
	if (b2f)
		filesize -= 2;	/* skip the B2F CRC bytes - implement later */
#endif

   StartHuff(lzhuf);
   for(i = 0; i < N - F; i++)
      lzhuf->text_buf[i] = ' ';

   r = N - F;

   for(count = 0; (long int)count < filesize; ) {
      pwait(NULL);
      c = DecodeChar(lzhuf);

	/* log (-1, "%d", c); */

      if(c < 256) {
         if(putc(c, lzhuf->oFile) == EOF) {
		return (do_errxit (i, lzhuf));
         }
         lzhuf->text_buf[r++] = c;
         r &= (N - 1);
         count++;
      } else {
         i = (r - DecodePosition(lzhuf) - 1) & (N - 1);
         j = c - 255 + THRESHOLD;
         for(k = 0; k < j; k++) {
            c = lzhuf->text_buf[(i + k) & (N - 1)];
            if(putc(c, lzhuf->oFile) == EOF) {
		return (do_errxit (i, lzhuf));
            }
            lzhuf->text_buf[r++] = c;
            r &= (N - 1);
            count++;
         }
      }
   }

   fclose(lzhuf->iFile);

     	fseek (lzhuf->oFile, 0L, SEEK_END);
     	/* log (-1, "size of decoded file is %ld", ftell(lzhuf->oFile)); */

   fclose(lzhuf->oFile);

	if (count > 0)
	{
		log (usock, "lzhuf uncompress %ld/%ld = %ld percent",
			lzhuf->iFileSize, count,
				(count - lzhuf->iFileSize) * 100L / count);
	}

#ifdef LZHSTAT
   if(count == 0)
      count  = 1;
   if(Current->output == Command->output)
      printf("lzhuf uncompress: %ld/%ld = %ld%%\n",
             lzhuf->iFileSize, count,
             (count - lzhuf->iFileSize) * 100L / count);
#endif
   return 0;
}

#ifdef HFDD
/*
 * 14Oct2006, Maiko, This is somewhat of a kludge (perhaps not)
 * for the HFDD server side. The j2send() call *fails* if I use
 * it with 'socketpair' sockets. It's actually the 'getpeername'
 * that fails within the j2send(), since the socketpair sockets
 * are really never connected, so 'peername' is never set, BUT
 * you can technically still do the 'send_mbuf' call. Until I
 * am more sure of how to properly fix this, this will do !
 * NOTE - this function does work for normal ax25 sockets. I
 * have tested my usual compressed forwarding, it works.
 */
static void hfddfixsend (int usock, char *buffer, int len)
{
	char *tptr = buffer;

	int cnt = len;
/*
	if (hfdd_debug)
	{
		log (-1, "sending %d bytes", len);

		pk232_dump (len, (unsigned char*)buffer);
	}
*/
	while (cnt > 0)
	{
		usputc (usock, *tptr);
		tptr++;
		cnt--;
	}

	usflush (usock);
}
#endif

/* Compress an input file, and write it to the output socket.
  Return 0 if error (and write note to the log),
  return 1 if all OK.
*/
/* 23Apr2008, Maiko (VE4KLM), added B2F flag for B2F considerations */
int send_yapp(int usock, struct fwd *f, char *subj, int b2f)
{
   #define SLEN 79                           // Maximum subject length
   int  oldmode;                             // Socket Mode holder.
   FILE *oFile;
#ifdef LZHDEBUG
   FILE *debug;
#endif

   char *ptr, *buffer;                       // buffer data.
   int  buffer_len;                          // buffer Length.

   int  x;                                   // misc counter.
   int  cnt;                                 // misc counter.
   int  rc;

/*   short b_checksum;                         // buffer checksum.*/
   short f_checksum;                         // file checksum.


#ifdef LZHDEBUG
    if (((debug  = fopen(tmpnam(NULL),"wb")) == NULLFILE)) {
        printf("Error opening input file.\n");
        return 0;
    }
#endif

   // Encode code.
      f->lzhuf->codesize = 0;
      f->lzhuf->getbuf   = 0;
      f->lzhuf->getlen   = 0;
      f->lzhuf->putbuf   = 0;
      f->lzhuf->putlen   = 0;
      f->lzhuf->code     = 0;
      f->lzhuf->len      = 0;

	/* 23Apr2008, Maiko (VE4KLM), Added 'b2f' flag for lzw decoding */
      rc = Encode(usock, f->iFile, f->oFile, f->lzhuf, b2f);
      if(rc) {
         log(usock, "lzhuf: Encode() error %d",rc);
#ifdef LZHDEBUG
         fclose(debug);
#endif
         return 0;
      }

      // Open the compressed data file.
      // We're going to read from the file and close it when we exit.
      oFile = fopen(f->oFile, "rb");

#ifdef LZHDEBUG
         printf("we opended %s for input.\n",  f->oFile);
#endif

      // Grab some space. Largest YAPP packet is 250+ bytes.
      buffer    = f->tmpBuffer;

      // Set the socket to Binary mode since we'll be sending Binary data.
      oldmode = sockmode(usock,SOCK_BINARY);

   // Send the subject buffer
      // The buffer is setup as follows:
      // Pos Data
      //   1 SOH
      //   2 Length of entire buffer ( 5 bytes + strlen(subject) )
      //   3 Null terminated Subject string.
      //   x Null terminated '0'-offset string.

      // Make sure that the subject strlen() is equal to or less than SLEN
      x = strlen(subj);
      if (x > SLEN) {
         x = SLEN;
         subj[SLEN] = '\0';
      }

#ifndef	REVISED

	buffer_len = x + 3;		/* now just subject + 2 NULLS + '0' character */

	ptr = buffer;

	*ptr++ = SOH;
	*ptr++ = buffer_len;
	strcpy (ptr, subj);
	ptr += x;
	*ptr++ = 0;
	*ptr++ = '0';
	*ptr++ = 0;
#else
      // length of subject + NULL + length of "     0" + NULL
      buffer_len = x + 1 + 6 + 1;

      // Build the buffer.
      buffer[0] = SOH;                       // buffer_Type
      buffer[1] = buffer_len;                // buffer_Len
      strcpy(&buffer[2], subj);              // Subject info.
      strcpy(&buffer[x+3], "     0");        // Always 0 for FBB Messages.
#endif

#ifndef HFDD
      // Now we can send it.
      // buffer_len + 2 ( for the first two bytes.
      j2send(usock, buffer, buffer_len+2, 0);
#else
	  hfddfixsend (usock, buffer, buffer_len + 2);
#endif

#ifdef LZHDEBUG
          fwrite(buffer, (size_t)(buffer_len+2), 1, debug);
#endif

   // Send the data buffers.
      f_checksum = 0;
      // fill buffer with data. Bytes 0 and 1 are reserved.
      while ((x = fread(&buffer[2], 1, 250, oFile)) > 0) {
         // prepare the buffer.
         buffer[0]  = STX;                         // buffer_Type
         buffer[1]  = x;                           // buffer_Len
/*         b_checksum = 0;*/
         for (cnt=0;cnt<x;cnt++) {
             f_checksum += buffer[2 + cnt];        // file checksum.
         }
/*         buffer[x + 2] = ((-b_checksum) & 0xff);   // Store b_checksum.*/

#ifndef	 HFDD
         // and send it.
         j2send(usock, buffer, (x + 2), 0);
#else
		 hfddfixsend (usock, buffer, x + 2);
#endif

#ifdef LZHDEBUG
         fwrite(buffer, (size_t)(x+2), 1, debug);
#endif

/*         f_checksum += b_checksum;*/
      } /* endwhile */

   // Send the EOT
      // Prepare the buffer.
      buffer[0] = EOT;                             // buffer_Type
      buffer[1] = ((-f_checksum) & 0xff);          // Checksum.

#ifndef HFDD
      // and send it.
      j2send(usock, buffer, 2, 0);
#else
	  hfddfixsend (usock, buffer, 2);
#endif

#ifdef LZHDEBUG
      fwrite(buffer, 2, 1, debug);
#endif

      // Terminate.
      fclose(oFile);
#ifdef LZHDEBUG
      fclose(debug);
#endif

      // Set the socket back to it's original mode.
      sockmode(usock,oldmode);
      return 1;
}


/* read a FBB-compressed file from a socket, and uncompress into a file.
   Return 0 if error (and write note to the log),
   Return 1 if all OK.
*/

/* 24Mar2008, Maiko (VE4KLM), added B2F flag for B2F considerations */

int recv_yapp(int usock, struct fwd *f, char **pzSubject, int32 Timeoutms, int b2f)
{
   int  recvcnt;
   FILE *iFile=NULLFILE;

   int  packet_type;
   int  packet_size;
   char packet_data[256];
   int  GetSubject;
   int  NoteDone;
   int  NoteError;
   int  checksumctr;
   int  checksum;
   int  rc;
   int  oldmode;                             // Socket Mode holder.

   // Set the socket to Binary mode since we'll be receiving Binary data.
      oldmode = sockmode(usock,SOCK_BINARY);

      GetSubject = TRUE;
      NoteDone   = FALSE;
      NoteError  = FALSE;
      checksum   = 0;

#ifdef HFDD
	if (hfdd_debug)
		log (-1, "recv_yapp recv timout %d", Timeoutms);
#endif

      while(!NoteDone) {
         // Get the data packets.
         j2alarm(Timeoutms);
         if ((packet_type = recvchar(usock)) == -1) {
             log(usock, EarlyDisconnect);
             NoteError = TRUE;
             break;
         }
         j2alarm(0);

#ifdef HFDD
	if (hfdd_debug)
		log (-1, "packet type [%x]", packet_type);
#endif
	/* log (-1, "packet type [%x]", packet_type); */

         if (GetSubject) {
             if (packet_type != SOH) {
			log (-1, "fbb protocol error, expecting SOH");
                 FBBerror(0,usock);
                 NoteError = TRUE;
                 break;
             }
         }
         else
         if ((packet_type != STX) && (packet_type != EOT))  {
			log (-1, "fbb protocol error, expecting STX or EOT");
             FBBerror(3,usock);
             NoteError = TRUE;
             break;
         }

         // Get the packet size.
         j2alarm(Timeoutms);
        if ((packet_size = recvchar(usock)) == -1)
		{
             log(usock, EarlyDisconnect);
             NoteError = TRUE;
             break;
		}
         j2alarm(0);
         if (!packet_size) packet_size=256;   /* 0x00 always means 256 */

#ifdef HFDD
		if (hfdd_debug)
			log (-1, "packet size [%d]", packet_size);
#endif
         if (packet_type == SOH) {
            // This is the subject. Reset the flag so we don't
            // come here again.
            GetSubject = FALSE;

            // Open the output file.
            iFile = fopen(f->iFile, "wb");

            // This is a subject packet.
            recvcnt = recvbuf(usock, (char *)&packet_data, packet_size, Timeoutms);
//		log (-1, "SOH - read %d of %d", recvcnt, packet_size);

            if(recvcnt == -1) {
               log(usock, EarlyDisconnect);
               // We've lost the connection ... close the data file.
               fclose(iFile);
               NoteDone  = TRUE;
               NoteError = TRUE;
               *pzSubject = NULLCHAR;
            } else
               *pzSubject = j2strdup(packet_data);  /* note this loses the offset digits */
         } /* end if */
         else
         if (packet_type == STX) {

#ifdef	DONT_COMPILE
			while (1)
			{
				int what2read = packet_size;

            // Validate the packet
            // and write it to the file.
            recvcnt = recvbuf(usock, (char *)&packet_data, what2read, Timeoutms);
//		log (-1, "STX - read %d of %d", recvcnt, packet_size);

			if (recvcnt == what2read)
				break;
//			log (-1, "do it again - read %d of %d bytes", recvcnt, what2read);

			what2read = what2read - recvcnt;

			}
#endif
            recvcnt = recvbuf(usock, (char *)&packet_data, packet_size, Timeoutms);
            if(recvcnt == -1) {
               log(usock, EarlyDisconnect);
               // We've lost the connection....
               NoteDone  = TRUE;
               NoteError = TRUE;
            } else {
               // Write to disk
               fwrite(packet_data, (size_t)recvcnt, 1, iFile);

               // add the data to the checksum count.
               for(checksumctr=0;checksumctr<packet_size;checksumctr++) {
                  checksum += packet_data[checksumctr];
               }
            }
         } /* end if */
         else
         if (packet_type == EOT) {

		/* 23Sep2006, Maiko, Ignore for now for flow testing */
            if (((-checksum) & 0xff) != (packet_size & 0xff)) {
			log (-1, "fbb protocol error, EOT checksum");
                 FBBerror(1,usock);
                 NoteError = TRUE;
                 /*break;  NoteDone is set next so we'll exit loop anyhow */
            }

            // We're done with this message.  Exit while loop and process it.
            NoteDone  = TRUE;
         } /* end if */
      } // End while !NoteDone

      if(iFile)
          fclose(iFile);   // Close the data file.

#ifdef HFDD
	if (hfdd_debug)
		log (-1, "close yap file [%s] decode ? %d", f->iFile, !NoteError);
#endif

	if (!NoteError)
	{
         f->lzhuf->codesize = 0;
         f->lzhuf->getbuf   = 0;
         f->lzhuf->getlen   = 0;
         f->lzhuf->putbuf   = 0;
         f->lzhuf->putlen   = 0;
         f->lzhuf->code     = 0;
         f->lzhuf->len      = 0;

/*
#ifdef	B2FTESTFILE
	strcpy(f->iFile, "/tmp/B2Ftestfile");
#endif
*/

/*	pscurproc ();   13Oct2009, Maiko, Log (debug) stack utilization */

	/* 24Mar2008, Maiko (VE4KLM), Added 'b2f' flag for lzw decoding */
         rc = Decode(usock, f->iFile, f->oFile, f->lzhuf, b2f);

         if (rc)
		log (usock, "lzhuf: Decode() error %d",rc), ++NoteError;
      }

#ifdef HFDD
		if (hfdd_debug)
			log (-1, "decode finished");
#endif
      // Set the socket back to it's orginal mode.
      sockmode(usock,oldmode);

      if(!NoteError)
         return 1;
      else
         return 0;
}

/* Receive a buffer from a socket, returning # chars read (or -1 if EOF/disconnect)
 */
static int
recvbuf(int s, char *buf, int len, int32 timeoutms) {
    int c;
    int cnt = 0;

    j2alarm(timeoutms);   /* assume long enough to read <len> bytes */
    while(len-- > 0){
        if((c = recvchar(s)) == EOF){
            cnt = -1;
            break;
        }
        if(buf != NULLCHAR)
            *buf++ = c;
        cnt++;
    }
    j2alarm(0);
    return cnt;
}

#endif /* defined(LZHUF) || defined(FBBCMP) */
