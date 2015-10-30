#ifndef _LZHUF_H
#define _LZHUF_H


// Added by MARTIN
#ifndef log
#define log no_log
#endif
void no_log(int s,const char *fmt, ...);

#include <stdio.h>

int pwait(void *event); // Some sort of signal to let other threads run?

#include <stdint.h>
typedef int32_t int32;
typedef int16_t int16;

#define errno 0
#define NULLFILE 0

struct lzhufstruct* AllocStruct();
void FreeStruct(struct lzhufstruct*);
// END

#if defined(LZHUF) || defined(FBBCMP)
//#include "mailbox.h"

typedef unsigned char uchar;

#define SOH       1
#define STX       2
#define EOT       4

// LZHUF variables.
#define N               2048    /* buffer size was 4096, */
#define F                 60    /* lookahead buffer size */
#define THRESHOLD          2
#define NIL                N    /* leaf of tree */

#define N_CHAR          (256 - THRESHOLD + F)   /* kinds of characters (character code = 0..N_CHAR-1) */
#define T               (N_CHAR * 2 - 1)        /* size of table */
#define R               (T - 1)                 /* position of root */
#define MAX_FREQ        0x8000                  /* updates tree when the */

struct lzhufdata {
   int           dad[N + 1];
   int           lson[N + 1];
   int           rson[N + 257];
   unsigned char text_buf[N + F - 1];
   unsigned int  freq[T + 1];
   int           prnt[T + N_CHAR];
   int           son[T];
};

struct lzhufstruct {
   FILE *iFile;
   FILE *oFile;
   struct lzhufdata    *data;
   int                 data_type;

   int                 *dad;
   int                 *lson;
   int                 *rson;
   unsigned char       *text_buf;
   unsigned int        *freq;
   int                 *prnt;
   int                 *son;

   long codesize;
   int  match_position;
   int  match_length;

   unsigned getbuf;
   unsigned char    getlen;
   unsigned putbuf;
   unsigned char    putlen;
   unsigned code;
   unsigned len;
   unsigned long    iFileSize;
   unsigned long    oFileSize;
};

/* 23Apr2008, Maiko (VE4KLM), Added flag for B2F considerations */
int Encode(int, char *, char *, struct lzhufstruct *, int);
/* 24Mar2008, Maiko (VE4KLM), Added flag for B2F considerations */
int Decode(int, char *, char *, struct lzhufstruct *, int);

#ifdef YAPP
/* 23Apr2008, Maiko (VE4KLM), Added flag for B2F considerations */
int send_yapp(int, struct fwd *, char *, int);
/* 24Mar2008, Maiko (VE4KLM), Added flag for B2F considerations */
int recv_yapp(int, struct fwd *, char **, int32, int);
int AllocDataBuffers(struct fwd *);
void FreeDataBuffers(struct fwd *);
#endif

#endif /* defined(LZHUF) || defined(FBBCMP) */
#endif /* _LZHUF_H */
