#include <Havoc/Connector.hpp>
#include <Havoc/Havoc.hpp>
#include <UserInterface/HavocUI.hpp>
#include <UserInterface/Dialogs/Connect.hpp>
#include <UserInterface/Widgets/TeamserverTabSession.h>
#include <QCryptographicHash>
#include <QMap>
#include <QBuffer>

Connector::Connector( Util::ConnectionInfo* ConnectionInfo )
{
    Teamserver   = ConnectionInfo;
    Socket       = new QWebSocket();
    auto Server  = "wss://" + Teamserver->Host + ":" + this->Teamserver->Port + "/havoc/";
    auto SslConf = Socket->sslConfiguration();

    /* verify the teamserver certificate by default; the connect dialog
     * offers an opt-in "Ignore SSL errors" toggle for the self-signed
     * certificates the teamserver ships with */
    if ( Teamserver->IgnoreSSLErrors )
    {
        SslConf.setPeerVerifyMode( QSslSocket::VerifyNone );
        Socket->setSslConfiguration( SslConf );

        QObject::connect( Socket, &QWebSocket::sslErrors, this, [&]( const QList<QSslError>& ) {
            Socket->ignoreSslErrors();
        } );
    }
    else
    {
        SslConf.setPeerVerifyMode( QSslSocket::VerifyPeer );
        Socket->setSslConfiguration( SslConf );
    }

    QObject::connect( Socket, QOverload<QAbstractSocket::SocketError>::of( &QWebSocket::error ), this, [&]( QAbstractSocket::SocketError )
    {
        ErrorString = Socket->errorString();

        /* if the connection never got established (refused, TLS failure,
         * closed during login) no disconnected() signal follows in most of
         * these cases: return to the connect dialog here so the client never
         * just sits dead or exits on a login-screen error. HavocUserInterface
         * only exists after the first teamserver package, so use the global
         * application's dbManager which is created unconditionally */
        if ( Packager == nullptr )
        {
            /* error and disconnected can both fire for the same failed
             * connect: only run the recovery-dialog flow once */
            if ( RecoveryDialogShown )
                return;
            RecoveryDialogShown = true;

            MessageBox( "Connection error", "Couldn't connect to the teamserver: " + ErrorString, QMessageBox::Critical );

            auto Connect = new UserInterface::Dialogs::Connect;

            Connect->TeamserverList = HavocApplication->dbManager->listTeamservers();
            Connect->passDB( HavocApplication->dbManager );
            Connect->setupUi( new QDialog );

            Connect->StartDialog( true );

            delete Connect;

            if ( HavocX::Connector == this )
                HavocX::Connector = nullptr;

            this->deleteLater();
        }
    } );

    QObject::connect( Socket, &QWebSocket::binaryMessageReceived, this, [&]( const QByteArray& Message )
    {
        auto Package = HavocSpace::Packager::DecodePackage( Message );

        if ( Package != nullptr )
        {
            if ( Packager )
                Packager->DispatchPackage( Package );

            delete Package;

            return;
        }

        spdlog::critical( "Got Invalid json" );
    } );

    QObject::connect( Socket, &QWebSocket::connected, this, [&]()
    {
        this->Packager = new HavocSpace::Packager;
        this->Packager->setTeamserver( this->Teamserver->Name );

        SendLogin();
    } );

    QObject::connect( Socket, &QWebSocket::disconnected, this, [&]()
    {
        /* the error handler already showed the recovery dialog and
         * scheduled this Connector's deletion: don't stack a second one */
        if ( RecoveryDialogShown )
            return;

        /* a null TabSession means the session never got initialized (the
         * login failed): the server already explained why via the
         * InitConnection::Error box, so don't stack a generic
         * "lost connection" box on top of it. same for a disconnect the
         * user explicitly requested from the menu */
        if ( ! UserDisconnect && HavocX::Teamserver.TabSession != nullptr )
            MessageBox( "Teamserver error", "Lost connection to the teamserver: " + Socket->errorString(), QMessageBox::Critical );

        Socket->close();

        /* tear down the stale teamserver tab; a successful reconnect
         * rebuilds it when the init package arrives */
        if ( HavocX::HavocUserInterface != nullptr && HavocX::Teamserver.TabSession != nullptr )
        {
            auto TabWidget = HavocX::HavocUserInterface->TeamserverTabWidget;
            auto Page      = HavocX::Teamserver.TabSession->PageWidget;
            auto Index     = TabWidget->indexOf( Page );

            if ( Index != -1 )
                TabWidget->removeTab( Index );

            HavocX::Teamserver.TabSession->deleteLater();
            HavocX::Teamserver.TabSession = nullptr;

            /* drop the session bookkeeping too: it references widgets owned
             * by the tab we just queued for deletion. release the python
             * task callbacks first — nothing will ever complete those tasks */
            if ( ! HavocX::Teamserver.Sessions.empty() )
            {
                auto GilState = PyGILState_Ensure();

                for ( auto& Session : HavocX::Teamserver.Sessions )
                {
                    for ( auto& [TaskID, Callback] : Session.TaskIDToPythonCallbacks )
                        Py_XDECREF( Callback );
                }

                PyGILState_Release( GilState );
            }

            HavocX::Teamserver.Sessions.clear();

            Page->deleteLater();
        }

        /* this Connector is dead once the slot returns; clear the global
         * so nothing touches a dangling pointer before reconnect */
        if ( HavocX::Connector == this )
            HavocX::Connector = nullptr;

        /* graceful handling: return to the connect dialog instead of
         * killing the whole client */
        auto Connect = new UserInterface::Dialogs::Connect;

        Connect->TeamserverList = HavocApplication->dbManager->listTeamservers();
        Connect->passDB( HavocApplication->dbManager );
        Connect->setupUi( new QDialog );

        /* StartDialog installs the new Connector and global teamserver
         * state itself on success; on cancel the client stays disconnected */
        Connect->StartDialog( true );

        delete Connect;

        /* schedule the deletion only after the dialog flow completed:
         * StartDialog runs nested exec() loops that would otherwise
         * process the deferred delete while this slot is still on the
         * stack */
        this->deleteLater();
    } );

    Socket->open( QUrl( Server ) );
}

bool Connector::Disconnect()
{
    if ( this->Socket != nullptr )
    {
        /* close the socket instead of severing its signals: the
         * disconnected handler then runs the normal teardown/reconnect
         * flow, and the flag keeps it from showing a "lost connection"
         * error box for a disconnect the user asked for */
        UserDisconnect = true;
        this->Socket->close();
        return true;
    }

    return false;
}

Connector::~Connector() noexcept
{
    delete this->Socket;

    /* the Connector owns the heap-allocated ConnectionInfo it was
     * constructed with (see Connect::StartDialog) */
    delete this->Teamserver;
}

void Connector::SendLogin()
{
    Util::Packager::Package Package;

    Util::Packager::Head_t Head;
    Util::Packager::Body_t Body;

    Head.Event              = Util::Packager::InitConnection::Type;
    Head.User               = this->Teamserver->User.toStdString();
    Head.Time               = CurrentTime().toStdString();

    Body.SubEvent           = Util::Packager::InitConnection::Login;
    Body.Info[ "User" ]     = this->Teamserver->User.toStdString();
    Body.Info[ "Password" ] = this->Teamserver->PasswordIsHashed ?
                              this->Teamserver->Password.toStdString() :
                              QCryptographicHash::hash( this->Teamserver->Password.toLocal8Bit(), QCryptographicHash::Sha3_256 ).toHex().toStdString();

    Package.Head = Head;
    Package.Body = Body;

    SendPackage( &Package );
}

void Connector::SendPackage( Util::Packager::PPackage Package )
{
    Socket->sendBinaryMessage( Packager->EncodePackage( *Package ).toJson( QJsonDocument::Compact ) );
}
