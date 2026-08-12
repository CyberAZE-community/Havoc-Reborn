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

    QObject::connect( Socket, &QWebSocket::binaryMessageReceived, this, [&]( const QByteArray& Message )
    {
        auto Package = HavocSpace::Packager::DecodePackage( Message );

        if ( Package != nullptr )
        {
            if ( ! Packager )
                return;

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

            Page->deleteLater();
        }

        /* this Connector is dead once the slot returns; clear the global
         * so nothing touches a dangling pointer before reconnect */
        if ( HavocX::Connector == this )
            HavocX::Connector = nullptr;

        this->deleteLater();

        /* graceful handling: return to the connect dialog instead of
         * killing the whole client */
        auto Connect = new UserInterface::Dialogs::Connect;

        Connect->TeamserverList = HavocX::HavocUserInterface->dbManager->listTeamservers();
        Connect->passDB( HavocX::HavocUserInterface->dbManager );
        Connect->setupUi( new QDialog );

        /* StartDialog installs the new Connector and global teamserver
         * state itself on success; on cancel the client stays disconnected */
        Connect->StartDialog( true );

        delete Connect;
    } );

    Socket->open( QUrl( Server ) );
}

bool Connector::Disconnect()
{
    if ( this->Socket != nullptr )
    {
        this->Socket->disconnect();
        return true;
    }

    return false;
}

Connector::~Connector() noexcept
{
    delete this->Socket;
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
